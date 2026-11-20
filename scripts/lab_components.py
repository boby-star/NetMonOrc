"""Owned external Component acceptance: real exporter, Agent, API and storage."""
import argparse
import copy
import io
import json
from pathlib import Path
import secrets
import subprocess
import tarfile
from lab import Lab, ROOT
from lab_replicas import CLIENT


def diagnostic_profile():
    return {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'DiagnosticProfile', 'metadata': {'name': 'external-once'}, 'spec': {
        'mode': 'fixed', 'budget': {'maxAdditionalChecks': 1, 'maxDuration': '60s', 'maxParallel': 1},
        'hypotheses': [{'id': 'responding'}, {'id': 'not-responding'}],
        'actions': [{'id': 'external-http', 'check': {'type': 'http', 'runner': 'blackbox', 'timeout': '2s', 'config': {'url': 'http://198.18.0.80:8080/external-diagnostic', 'expectedStatus': [200]}},
                     'expectedOutcomes': {'responding': 'success', 'not-responding': 'failure'}, 'placement': {'agentRefs': ['branch'], 'zones': ['office'], 'strategy': 'fixed'}}],
        'fixed': {'steps': [{'id': 'request', 'actionRef': 'external-http'}]}}}


class ComponentLab(Lab):
    def compose(self, *args, input=None, env=None):
        override = self.work/'components.compose.json'
        if not override.exists(): return super().compose(*args, input=input, env=env)
        return subprocess.run(['docker', 'compose', '-p', self.project, '-f', str(ROOT/'compose.yaml'), '-f', str(override), *args], cwd=ROOT, env={**self.env, **(env or {})}, input=input, stdin=subprocess.DEVNULL if input is None else None, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True).stdout

    def prepare_components(self):
        if self.work.exists(): raise RuntimeError('Use a new owned project; existing data is preserved')
        self.prepare()
        command = json.loads(self.compose('config', '--format', 'json'))['services']['controller']['command']
        command.append('-agent-labels-file=/run/netmonorc/agent-labels.json')
        services = {'controller': {'command': command}}
        for name, namespace, volume in [('office-exporter', 'office-net', 'branch-config'), ('server-exporter', 'server-net', 'dc-config')]:
            services[name] = {'image': 'netmonorc-research:local', 'entrypoint': ['/usr/local/bin/blackbox_exporter'], 'command': ['--config.file=/run/netmonorc/external-blackbox.json', '--web.listen-address=127.0.0.1:9115', '--timeout-offset=0'],
                              'network_mode': 'service:'+namespace, 'depends_on': {namespace: {'condition': 'service_healthy'}}, 'user': '10001:10001', 'read_only': True, 'cap_drop': ['ALL'], 'security_opt': ['no-new-privileges:true'],
                              'pids_limit': 32, 'mem_limit': '64m', 'volumes': [volume+':/run/netmonorc:ro']}
        (self.work/'components.compose.json').write_text(json.dumps({'services': services}))
        bundle = (ROOT/'deployment/lab/monitor.yaml').read_text()+'\n---\n'+(ROOT/'deployment/lab/adaptive-confirmed.yaml').read_text()+'\n---\n'+json.dumps(diagnostic_profile())
        modules = self.compose('run', '--rm', '--no-deps', '-T', '--entrypoint', '/usr/local/bin/netmonorc', 'controller', 'component-config', '-f', '-', input=bundle.encode())
        assert json.loads(modules)['modules']
        labels = json.dumps({'branch': {'role': 'probe'}, 'dc': {'role': 'probe'}}).encode()
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode='w') as tar:
            for name, payload in [('controller/agent-labels.json', labels), ('branch/external-blackbox.json', modules), ('dc/external-blackbox.json', modules)]:
                item = tarfile.TarInfo(name); item.size = len(payload); item.mode = 0o600; item.uid = item.gid = 10001; tar.addfile(item, io.BytesIO(payload))
        self.compose('run', '--rm', '--no-deps', '-T', 'init', input=archive.getvalue())
        print('Prepared offline content-addressed modules and isolated external exporter services')


def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'component-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'component-admin', 'password': password}); password = ''
    assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    monitor_path = '/api/v1/resources/Monitor/research-web'; component_path = '/api/v1/resources/Component/local-blackbox'
    original = request('GET', monitor_path, **auth)
    manifest = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'Component', 'metadata': {'name': 'local-blackbox'}, 'spec': {'driver': 'blackbox', 'agentSelector': {'matchLabels': {'role': 'probe'}}, 'endpoint': 'http://127.0.0.1:9115', 'moduleSet': 'standard-network', 'enabled': True}}
    for field, value in [('mode', 'managed'), ('endpoint', 'http://8.8.8.8:9115'), ('moduleSet', 'arbitrary')]:
        invalid = copy.deepcopy(manifest); invalid['spec'][field] = value
        result = request('POST', '/api/v1/config/validate', {'manifests': [invalid]}, **auth)
        assert result['status'] == 200 and not result['body']['valid'], result
    assert request('POST', '/api/v1/config/plan', {'manifests': [manifest]}, **auth)['status'] == 200
    created = request('POST', '/api/v1/resources/Component', manifest, **auth)
    assert created['status'] == 201, created
    def revision_state(want):
        def ready():
            response = request('GET', monitor_path, **auth)
            if response['status'] != 200: return None
            revision = response['body']['status']['desiredRevision']
            current = request('GET', '/api/v1/monitors/research-web/assessment', **auth)
            assessment = current['body'] if current['status'] == 200 else {}
            return response if assessment.get('revision') == revision and assessment.get('state') == want else None
        return lab.wait(ready, 'Component revision '+want, seconds=70)
    def component_state(want):
        def ready():
            source = request('GET', monitor_path, **auth)
            response = request('GET', '/api/v1/agents', **auth)
            if source['status'] != 200 or response['status'] != 200: return None
            revision = source['body']['status']['desiredRevision']
            rows = [{'agentId': a['agentId'], **c} for a in response['body']['items'] for c in a.get('components', []) if c['componentRef'] == 'local-blackbox' and c['monitorId'] == 'research-web']
            if len(rows) != 2 or not all(c['revision'] == revision and c['state'] == want for c in rows): return None
            if want != 'disabled' and not all(c['checkedAt'] for c in rows): return None
            return rows
        return lab.wait(ready, 'both Component readiness reports '+want, seconds=45)
    def observations(revision, since='1970-01-01T00:00:00Z'):
        assert len(revision) == 64 and all(c in '0123456789abcdef' for c in revision)
        return json.loads(lab.sql("SELECT COALESCE(json_agg(x),'[]')::text FROM (SELECT DISTINCT agent_id,check_id,payload->>'execution' AS execution,payload->>'outcome' AS outcome,payload->>'reason' AS reason FROM nmo_observations WHERE revision='"+revision+"' AND diagnostic_task_id='' AND finished_at>'"+since+"'::timestamptz ORDER BY agent_id,check_id) x"))
    active = revision_state('healthy'); revision = active['body']['status']['desiredRevision']
    assert active['etag'] == original['etag'] and revision != original['body']['status']['desiredRevision']
    good = observations(revision)
    assert len(good) == 6 and all(x['execution'] == 'completed' and x['outcome'] == 'success' for x in good), good
    duplicate = copy.deepcopy(manifest); duplicate['metadata']['name'] = 'overlap'
    rejected = request('POST', '/api/v1/resources/Component', duplicate, **auth)
    assert rejected['status'] == 422, rejected
    assert request('POST', '/api/v1/resources/DiagnosticProfile', diagnostic_profile(), **auth)['status'] == 201
    revision_state('healthy'); initial_ready = component_state('ready'); request_id = secrets.token_hex(16)
    def start():
        response = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': 'external-once', 'requestId': request_id}, **auth)
        assert response['status'] in [200, 202, 409, 503], response
        return response['body'] if response['status'] in [200, 202] else None
    run = lab.wait(start, 'external diagnostic start')['id']
    def terminal():
        response = request('GET', '/api/v1/diagnostics/'+run, **auth)
        return response['body'] if response['status'] == 200 and response['body']['state'] not in ['created', 'running'] else None
    detail = lab.wait(terminal, 'external diagnostic completion')
    assert detail['state'] == 'completed' and detail['remainingHypotheses'] == ['responding'] and detail['budgetUsed']['additionalChecks'] == 1, detail
    # Stopping the declared external services proves the Agent does not silently
    # fall back to its bundled executable, which remains installed and usable.
    lab.compose('stop', 'office-exporter', 'server-exporter')
    since = lab.sql("SELECT to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"')")
    revision_state('unknown'); unavailable = component_state('unavailable')
    def outage_records():
        rows = observations(revision, since)
        return rows if len(rows) == 6 and all(x['execution'] == 'execution_failed' and x['outcome'] == 'unknown' and x['reason'] == 'runner_unavailable' for x in rows) else None
    outage = lab.wait(outage_records, 'six external outage results', seconds=60)
    blocked_request_id = secrets.token_hex(16)
    def blocked_start():
        response = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': 'external-once', 'requestId': blocked_request_id}, **auth)
        assert response['status'] in [200, 202, 409, 503], response
        return response['body'] if response['status'] in [200, 202] else None
    blocked_run = lab.wait(blocked_start, 'manual run during confirmed Component outage')['id']
    def blocked_terminal():
        response = request('GET', '/api/v1/diagnostics/'+blocked_run, **auth)
        return response['body'] if response['status'] == 200 and response['body']['state'] not in ['created', 'running'] else None
    blocked_detail = lab.wait(blocked_terminal, 'unavailable Component run without physical budget')
    assert blocked_detail['state'] == 'inconclusive' and blocked_detail['budgetUsed']['additionalChecks'] == 0, blocked_detail
    blocked_tasks = request('GET', '/api/v1/diagnostics/'+blocked_run+'/tasks', **auth)['body']['items']
    assert blocked_tasks == [], blocked_tasks
    lab.compose('start', 'office-exporter', 'server-exporter'); revision_state('healthy'); recovered = component_state('ready')
    current = request('GET', component_path, **auth); manifest['spec']['enabled'] = False
    assert request('PUT', component_path, manifest, etag=current['etag'], **auth)['status'] == 200
    disabled = revision_state('unknown'); disabled_revision = disabled['body']['status']['desiredRevision']
    lab.wait(lambda: len(observations(disabled_revision)) == 6, 'six disabled binding results')
    assert all(x['reason'] == 'runner_unavailable' for x in observations(disabled_revision))
    disabled_status = component_state('disabled')
    lab.compose('restart', 'controller')
    restored = revision_state('unknown'); assert restored['body']['status']['desiredRevision'] == disabled_revision
    current = request('GET', component_path, **auth); manifest['spec']['enabled'] = True
    assert request('PUT', component_path, manifest, etag=current['etag'], **auth)['status'] == 200
    revision_state('healthy'); component_state('ready')
    current = request('GET', component_path, **auth)
    assert request('DELETE', component_path, etag=current['etag'], **auth)['status'] == 204
    final = revision_state('healthy')
    assert final['etag'] == original['etag'] and final['body']['status']['desiredRevision'] == original['body']['status']['desiredRevision']
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'trustedHTTPSAndMTLS': True, 'realPostgreSQLAndExporters': True, 'regularExternal': good, 'diagnosticExternal': detail, 'initialReadiness': initial_ready, 'outageReadiness': unavailable, 'blockedDiagnostic': blocked_detail, 'blockedTasks': blocked_tasks, 'recoveredReadiness': recovered, 'disabledReadiness': disabled_status, 'outageWithoutFallback': outage, 'disabledBindingUnknown': True, 'restartPreservedRevision': True, 'deletionRestoredLegacyRunner': True, 'monitorSourceVersionUnchanged': True, 'invalidBindingsRejected': True, 'productionCLIModulePreparation': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'up', 'verify', 'down']); parser.add_argument('--project', required=True); parser.add_argument('--prefix', required=True); parser.add_argument('--output')
    args = parser.parse_args(); lab = ComponentLab(args.project, args.prefix)
    if args.action == 'prepare': lab.prepare_components()
    elif args.action == 'up': lab.up()
    elif args.action == 'down': lab.require(); lab.compose('down'); print('Stopped; .local, PKI and named volumes preserved')
    else:
        if not args.output: parser.error('--output required for verify')
        path = Path(args.output)
        if path.exists(): raise FileExistsError('Use a new output path')
        result = verify(lab); path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
        with path.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
        path.chmod(0o600); print('PASS: Component readiness, zero-task outage admission, external probes, disabled/restart and recovery; secrets omitted')
