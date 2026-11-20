"""Owned execution fallback: original evidence, zone coverage and missed source."""
import argparse
import io
import json
from pathlib import Path
import secrets
import subprocess
import tarfile
from lab import Lab, ROOT
from lab_replicas import CLIENT


class FallbackLab(Lab):
    def compose(self, *args, input=None, env=None):
        override = self.work/'fallback.compose.json'
        if not override.exists(): return super().compose(*args, input=input, env=env)
        return subprocess.run(['docker', 'compose', '-p', self.project, '-f', str(ROOT/'compose.yaml'), '-f', str(override), *args], cwd=ROOT, env={**self.env, **(env or {})}, input=input, stdin=subprocess.DEVNULL if input is None else None, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True).stdout

    def prepare_fallback(self, strategy):
        if self.work.exists(): raise RuntimeError('Use a new owned project')
        self.prepare()
        command = json.loads(self.compose('config', '--format', 'json'))['services']['controller']['command']
        if strategy == 'same-zone': command = ['-slots=branch=office,dc=office' if x.startswith('-slots=') else x for x in command]
        command.append('-agent-labels-file=/run/netmonorc/agent-labels.json')
        (self.work/'fallback.compose.json').write_text(json.dumps({'services': {'controller': {'command': command}}}))
        manifest = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'Monitor', 'metadata': {'name': 'research-web'}, 'spec': {
            'target': {'address': '198.18.0.80'}, 'placement': {'strategy': 'fixed', 'agentRefs': ['branch'], 'replicas': 1},
            'checks': [{'id': 'http', 'type': 'http', 'runner': 'blackbox', 'interval': '10s', 'timeout': '2s', 'config': {'url': 'http://198.18.0.80:8080/', 'expectedStatus': [200]},
                        'fallback': {'enabled': True, 'on': ['runner_unavailable', 'local_execution_error', 'agent_unreachable', 'config_rejected'], 'strategy': strategy, 'maxAttempts': 2}}],
            'assessment': {'requiredZones': ['office'], 'freshness': {'maxAge': '20s', 'maxSkew': '15s'}, 'checks': [{'id': 'http', 'weight': 1, 'required': True}]},
            'diagnostics': {'enabled': False, 'profileRef': 'web-lab'}}}
        labels = {'branch': {'role': 'primary'}, 'dc': {'role': 'spare'}}
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode='w') as tar:
            for name, value in [('controller/monitor.yaml', manifest), ('controller/agent-labels.json', labels)]:
                payload = json.dumps(value).encode(); item = tarfile.TarInfo(name); item.size = len(payload); item.mode = 0o600; item.uid = item.gid = 10001; tar.addfile(item, io.BytesIO(payload))
        self.compose('run', '--rm', '--no-deps', '-T', 'init', input=archive.getvalue())
        (self.work/'fallback-strategy').write_text(strategy)
        print('Prepared fixed primary and trusted spare for '+strategy)

    def up(self):
        self.require();self.compose('up','-d','--wait','--wait-timeout','90')
        self.wait(lambda:self.assessment().get('state')=='healthy','fixed primary healthy; spare intentionally has no regular checks',seconds=60)


def verify(lab):
    lab.require(); strategy = (lab.work/'fallback-strategy').read_text()
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'fallback-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'fallback-admin', 'password': password}); password = ''; assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}; path = '/api/v1/resources/Monitor/research-web'
    original = request('GET', path, **auth)
    def assessment(want):
        def ready():
            resource = request('GET', path, **auth); response = request('GET', '/api/v1/monitors/research-web/assessment', **auth)
            if resource['status'] != 200 or response['status'] != 200: return None
            a = response['body']; return a if a['revision'] == resource['body']['status']['desiredRevision'] and a['state'] == want else None
        return lab.wait(ready, 'exact fallback revision '+want, seconds=75)
    def runs():
        response = request('GET', '/api/v1/fallback-runs?monitor=research-web&limit=100', **auth)
        assert response['status'] == 200, response
        return response['body']['items']
    def completed(cause):
        return next((r for r in runs() if r['cause'] == cause and r['state'] == 'completed' and len(r['tasks']) == 1 and r['tasks'][0]['observationId']), None)
    try:
        lab.inject('http'); negative = assessment('down'); assert runs() == [], 'target 503 triggered fallback'
        lab.inject('healthy'); assessment('healthy')
        component = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'Component', 'metadata': {'name': 'primary-off'}, 'spec': {'driver': 'blackbox', 'agentSelector': {'matchLabels': {'role': 'primary'}}, 'endpoint': 'http://127.0.0.1:9115', 'enabled': False}}
        assert request('POST', '/api/v1/resources/Component', component, **auth)['status'] == 201
        root = lab.wait(lambda: completed('runner_unavailable'), 'spare recovers technical execution', seconds=60)
        parent = request('GET', '/api/v1/observations/'+root['originObservationId'], **auth)
        child = request('GET', '/api/v1/observations/'+root['tasks'][0]['observationId'], **auth)
        assert parent['status'] == child['status'] == 200
        assert parent['body']['executionStatus'] == 'execution_failed' and parent['body']['agentId'] == 'branch'
        assert child['body']['executionStatus'] == 'completed' and child['body']['probeOutcome'] == 'success' and child['body']['agentId'] == 'dc'
        assert child['body']['fallbackRunId'] == root['id'] and child['body']['fallbackTaskId'] == root['tasks'][0]['id']
        result = assessment('healthy' if strategy == 'same-zone' else 'unknown')
        assert child['body']['networkZone'] == ('office' if strategy == 'same-zone' else 'server')
        if strategy == 'same-zone': assert child['body']['observationId'] in result['evidenceIds']
        else: assert child['body']['observationId'] not in result['evidenceIds']
        detail = request('GET', '/api/v1/fallback-runs/'+root['id'], **auth); assert detail['status'] == 200 and detail['body']['usedAttempts'] == 1
        current = request('GET', '/api/v1/resources/Component/primary-off', **auth)
        assert request('DELETE', '/api/v1/resources/Component/primary-off', etag=current['etag'], **auth)['status'] == 204
        assessment('healthy')
        lab.compose('stop', 'branch')
        missing = lab.wait(lambda: completed('agent_unreachable'), 'fallback for missed source without invented Observation', seconds=90)
        assert missing['originObservationId'] == '' and missing['originAgent'] == 'branch' and missing['usedAttempts'] == 1
        missing_state = assessment('healthy' if strategy == 'same-zone' else 'unknown')
        lab.compose('start', 'branch'); assessment('healthy')
        # Current roots may be multiple regular intervals; no correlation can
        # exceed its persisted cap or reassign the same spare twice.
        for r in runs(): assert r['usedAttempts'] <= r['maxAttempts'] <= 2 and len({t['agent'] for t in r['tasks']}) == len(r['tasks'])
        final = request('GET', path, **auth)
        assert final['etag'] == original['etag'] and final['body']['status']['desiredRevision'] == original['body']['status']['desiredRevision']
        assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
        return {'status': 'passed', 'strategy': strategy, 'trustedHTTPSAndMTLS': True, 'realPostgreSQLAndProbes': True, 'negativeTargetNoFallback': negative, 'technicalRoot': root, 'original': parent['body'], 'fallback': child['body'], 'assessmentAfterFallback': result, 'missingAgentRun': missing, 'missingAgentAssessment': missing_state, 'sourceVersionUnchanged': True, 'restoredHealthy': True, 'browserInteractionTested': False, 'credentialsOmitted': True}
    finally:
        lab.inject('healthy'); lab.compose('start', 'branch')


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('action', choices=['prepare', 'up', 'verify', 'down']); p.add_argument('--project', required=True); p.add_argument('--prefix', required=True); p.add_argument('--strategy', choices=['same-zone', 'different-zone'], default='same-zone'); p.add_argument('--output')
    a = p.parse_args(); lab = FallbackLab(a.project, a.prefix)
    if a.action == 'prepare': lab.prepare_fallback(a.strategy)
    elif a.action == 'up': lab.up()
    elif a.action == 'down': lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not a.output: p.error('--output required')
        path = Path(a.output)
        if path.exists(): raise FileExistsError('Use a new output path')
        result = verify(lab); path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
        with path.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
        path.chmod(0o600); print('PASS: '+result['strategy']+' technical fallback, original provenance, missed Agent and coverage rules; secrets omitted')
