"""Reuse an authenticated same-zone fallback as diagnostic baseline, with CLI export."""
import argparse
import io
import json
from pathlib import Path
import secrets
import subprocess
import tarfile
from lab_fallback import FallbackLab
from lab_replicas import CLIENT


def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project; existing users preserved')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'fallback-proof-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'fallback-proof-admin', 'password': password}); password = ''; assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    path = '/api/v1/resources/Monitor/research-web'
    current = request('GET', path, **auth)
    check = current['body']['manifest']['spec']['checks'][0]
    action_check = {k: v for k, v in check.items() if k in ['type', 'runner', 'timeout', 'config', 'extract']}
    profile = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'DiagnosticProfile', 'metadata': {'name': 'fallback-baseline'}, 'spec': {
        'mode': 'adaptive', 'adaptive': {}, 'budget': {'maxAdditionalChecks': 2, 'maxDuration': '60s', 'maxParallel': 1},
        'hypotheses': [{'id': 'responding'}, {'id': 'not-responding'}],
        'actions': [{'id': 'same-http', 'check': action_check, 'expectedOutcomes': {'responding': 'success', 'not-responding': 'failure'}, 'placement': {'agentRefs': ['dc'], 'zones': ['office'], 'strategy': 'fixed'}}]}}
    created = request('POST', '/api/v1/resources/DiagnosticProfile', profile, **auth)
    assert created['status'] == 201, created
    current = request('GET', path, **auth); manifest = current['body']['manifest']
    manifest['spec']['assessment']['freshness']['maxAge'] = '60s'
    manifest['spec']['diagnostics'] = {'enabled': True, 'profileRef': 'fallback-baseline', 'when': 'false'}
    update = request('PUT', path, manifest, etag=current['etag'], **auth); assert update['status'] == 200
    revision = update['body']['status']['desiredRevision']
    lab.wait(lambda: lab.assessment().get('revision') == revision and lab.assessment().get('state') == 'healthy', 'healthy exact revision before outage', seconds=60)
    evidence = None
    try:
        lab.compose('stop', 'branch')
        def recovered():
            roots = request('GET', '/api/v1/fallback-runs?monitor=research-web&limit=100', **auth)
            assert roots['status'] == 200
            return next((r for r in roots['body']['items'] if r['revision'] == revision and r['cause'] == 'agent_unreachable' and r['state'] == 'completed' and r['tasks'] and r['tasks'][0]['observationId']), None)
        root = lab.wait(recovered, 'same-zone fallback from spare', seconds=90)
        lab.wait(lambda: request('GET','/api/v1/monitors/research-web/assessment',**auth)['body'].get('evidenceIds',[]).count(root['tasks'][0]['observationId'])>0,'fallback admitted to current assessment before diagnostic',seconds=10)
        request_id = secrets.token_hex(16)
        def start():
            response = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': 'fallback-baseline', 'reason': 'manual', 'requestId': request_id}, **auth)
            if response['status'] == 409: return None
            assert response['status'] == 202, response
            return response['body']['id']
        session_id = lab.wait(start, 'fresh manual diagnostic', seconds=30)
        def finished():
            response = request('GET', '/api/v1/diagnostics/'+session_id, **auth)
            return response['body'] if response['status'] == 200 and response['body']['state'] in ['completed', 'inconclusive', 'cancelled'] else None
        session = lab.wait(finished, 'baseline-only diagnosis', seconds=30)
        assert session['state'] == 'completed' and session['remainingHypotheses'] == ['responding'], session
        assert session['budgetUsed']['additionalChecks'] == 0, session
        assert request('GET', '/api/v1/diagnostics/'+session_id+'/tasks?limit=200', **auth)['body']['items'] == []
        baseline = session['explanation']['baselineEvidence']; assert len(baseline) == 1, baseline
        proof = baseline[0]
        assert proof['agentId'] == 'dc' and proof['zone'] == 'office' and proof['fallbackOriginAgentId'] == 'branch'
        assert proof['observationId'] == proof['fallbackTaskId'] and proof['fallbackRunId']
        observation = request('GET', '/api/v1/observations/'+proof['observationId'], **auth); assert observation['status'] == 200
        body = observation['body']
        assert body['agentId'] == 'dc' and body['fallbackOriginAgentId'] == 'branch' and body['fallbackRunId'] == proof['fallbackRunId'] and body['fallbackTaskId'] == proof['fallbackTaskId']
        root_view = request('GET', '/api/v1/fallback-runs/'+proof['fallbackRunId'], **auth)['body']
        assert root_view['originObservationId'] == '' and root_view['usedAttempts'] == 1, root_view
        token = request('POST', '/api/v1/auth/tokens', {'name': 'fallback-export', 'scopes': ['resources:read', 'diagnostics:read']}, **auth); assert token['status'] == 201
        context = '/tmp/fallback-proof-cli/context.json'
        def cli(*args, input=None): return lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc', *args, input=input)
        try:
            cli('login', '--server', 'https://localhost:8443', '--ca-file', '/run/netmonorc/ca.crt', '--context-file', context, '--token-stdin', input=token['body']['token'].encode())
            cli('diagnostics', 'export', '--context-file', context, '--output', '/data/fallback-proof-export.json', session_id)
            container=lab.compose('ps','--quiet','controller').decode().strip()
            archive=subprocess.check_output(['docker','cp',container+':/data/fallback-proof-export.json','-'])
            with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
                member=next(m for m in tar.getmembers() if m.isfile())
                package=json.load(tar.extractfile(member))
            assert package['complete'] and package['baselineEvidenceKnown'] and package['tasks'] == []
            assert len(package['observations']) == 1 and package['observations'][0]['fallbackTaskId'] == proof['fallbackTaskId']
        finally:
            cli('logout', '--context-file', context)
            request('DELETE', '/api/v1/auth/tokens/'+token['body']['id'], **auth)
        evidence = {'status': 'passed', 'root': root_view, 'session': session, 'fallbackObservation': body, 'cliExport': package, 'physicalAgentPreserved': True, 'noAdditionalDiagnosticTasks': True, 'noInventedInitialObservation': True, 'browserInteractionTested': False, 'credentialsOmitted': True}
    finally:
        lab.compose('start', 'branch')
    lab.wait(lambda: lab.assessment().get('state') == 'healthy' and lab.assessment().get('revision') == revision, 'primary restored healthy', seconds=60)
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    evidence['primaryRestored'] = True
    return evidence


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('action', choices=['prepare', 'verify', 'down']); p.add_argument('--project', required=True); p.add_argument('--prefix', required=True); p.add_argument('--output')
    a = p.parse_args(); lab = FallbackLab(a.project, a.prefix)
    if a.action == 'prepare': lab.prepare_fallback('same-zone')
    elif a.action == 'down': lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not a.output: p.error('--output required')
        output = Path(a.output)
        if output.exists(): raise FileExistsError('Use a new output path')
        lab.up(); result = verify(lab); output.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
        with output.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
        output.chmod(0o600); print('PASS: fallback physical proof -> zero-task diagnosis -> complete production CLI export')
