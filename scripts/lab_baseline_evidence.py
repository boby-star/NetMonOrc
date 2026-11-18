"""Verify regular diagnostic inputs and a zero-task production CLI evidence export."""
import argparse
import io
import json
from pathlib import Path
import secrets
import subprocess
import tarfile
from lab import Lab
from lab_replicas import CLIENT


def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users') != '0':
        raise RuntimeError('Use a fresh owned project; existing users are preserved')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'baseline-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'baseline-admin', 'password': password})
    password = ''
    assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    source = request('GET', '/api/v1/resources/Monitor/research-web', **auth)
    assert source['status'] == 200
    check = next(c for c in source['body']['manifest']['spec']['checks'] if c['id'] == 'http')
    action_check = {k: v for k, v in check.items() if k in ['type', 'runner', 'timeout', 'config', 'extract']}
    profile = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'DiagnosticProfile', 'metadata': {'name': 'baseline-only'}, 'spec': {
        'mode': 'adaptive', 'adaptive': {}, 'budget': {'maxAdditionalChecks': 2, 'maxDuration': '60s', 'maxParallel': 1},
        'hypotheses': [{'id': 'responding'}, {'id': 'not-responding'}],
        'actions': [{'id': 'same-http', 'check': action_check, 'expectedOutcomes': {'responding': 'success', 'not-responding': 'failure'}, 'placement': {'agentRefs': ['branch'], 'zones': ['office'], 'strategy': 'fixed'}}]}}
    assert request('POST', '/api/v1/resources/DiagnosticProfile', profile, **auth)['status'] == 201
    lab.wait(lambda: request('GET', '/api/v1/auth/me', **auth)['status'] == 200 and lab.assessment().get('state') == 'healthy', 'healthy runtime after profile creation')
    request_id = secrets.token_hex(16)
    def start():
        response = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': 'baseline-only', 'requestId': request_id}, **auth)
        assert response['status'] in [200, 202, 409, 503], response['status']
        return response['body'] if response['status'] in [200, 202] else None
    run = lab.wait(start, 'idempotent manual start')['id']
    def terminal():
        response = request('GET', '/api/v1/diagnostics/'+run, **auth)
        return response['body'] if response['status'] == 200 and response['body']['state'] not in ['created', 'running'] else None
    detail = lab.wait(terminal, 'baseline-only terminal diagnostic')
    assert detail['state'] == 'completed' and detail['remainingHypotheses'] == ['responding']
    assert detail['budgetUsed']['additionalChecks'] == 0 and detail['explanation']['decisions'] == []
    baseline = detail['explanation']['baselineEvidence']
    assert len(baseline) == 1 and baseline[0]['actionId'] == 'same-http' and baseline[0]['checkId'] == 'http'
    proof = request('GET', '/api/v1/observations/'+baseline[0]['observationId'], **auth)
    assert proof['status'] == 200 and not proof['body'].get('diagnosticSessionId') and not proof['body'].get('diagnosticTaskId')
    assert proof['body']['finishedAt'] == baseline[0]['finishedAt'] and proof['body']['configRevision'] == detail['configRevision']
    issued = request('POST', '/api/v1/auth/tokens', {'name': 'baseline-export', 'scopes': ['diagnostics:read', 'resources:read']}, **auth)
    assert issued['status'] == 201
    token_id = issued['body']['id']
    try:
        # Private context travels via stdin into this fresh project's config volume.
        context = {'server': 'https://localhost:8443', 'token': issued['body']['token'], 'caFile': '/run/netmonorc/ca.crt'}
        payload = json.dumps(context).encode()
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode='w') as tar:
            item = tarfile.TarInfo('controller/baseline-context.json'); item.size = len(payload); item.mode = 0o600; item.uid = item.gid = 10001
            tar.addfile(item, io.BytesIO(payload))
        lab.compose('run', '--rm', '--no-deps', '-T', 'init', input=archive.getvalue())
        output = lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc', 'diagnostics', 'export', '--context-file', '/run/netmonorc/baseline-context.json', '--output', '/data/baseline-evidence.json', run).decode()
        assert 'complete=true' in output and 'baselineEvidenceKnown=true' in output
        container = lab.compose('ps', '--quiet', 'controller').decode().strip()
        exported = subprocess.check_output(['docker', 'cp', container+':/data/baseline-evidence.json', '-'])
        with tarfile.open(fileobj=io.BytesIO(exported)) as tar:
            member = next(m for m in tar.getmembers() if m.isfile())
            package = json.load(tar.extractfile(member))
        assert package['complete'] and package['baselineEvidenceKnown'] and package['tasks'] == []
        assert len(package['observations']) == 1 and package['observations'][0]['observationId'] == baseline[0]['observationId']
    finally:
        assert request('DELETE', '/api/v1/auth/tokens/'+token_id, **auth)['status'] == 204
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'trustedHTTPS': True, 'realPostgreSQLAndAgentProbes': True, 'state': detail['state'], 'usedChecks': 0, 'physicalTasks': 0, 'baselineEvidence': baseline, 'productionCLIExport': {'complete': package['complete'], 'baselineEvidenceKnown': package['baselineEvidenceKnown'], 'observationCount': len(package['observations'])}, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project', required=True); parser.add_argument('--prefix', required=True); parser.add_argument('--output', required=True)
    args = parser.parse_args(); path = Path(args.output)
    if path.exists(): raise FileExistsError('Use a new output path')
    result = verify(Lab(args.project, args.prefix))
    path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    with path.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
    path.chmod(0o600)
    print('PASS: persisted regular baseline, zero-task diagnosis and production CLI evidence export; secrets omitted')
