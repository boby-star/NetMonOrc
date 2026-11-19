"""Accept live estimated-byte admission and durable public accounting."""
import argparse
import copy
import json
from pathlib import Path
import secrets
from lab import Lab
from lab_replicas import CLIENT


def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users') != '0':
        raise RuntimeError('Use a fresh owned project; preserve existing users')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'byte-budget-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'byte-budget-admin', 'password': password}); password = ''
    assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    source = request('GET', '/api/v1/resources/Monitor/research-web', **auth)['body']['manifest']
    http = next(c for c in source['spec']['checks'] if c['id'] == 'http')
    check = {k: copy.deepcopy(v) for k, v in http.items() if k in ['type', 'runner', 'timeout', 'config']}
    check['config']['url'] = 'http://198.18.0.80:8080/byte-budget'
    check['config']['maxBodyBytes'] = 16384
    estimate = 32768 + 16384 + len(check['config']['url'])
    cases = {}
    for name, budget, want in [('too-small', 1, 0), ('one-attempt', estimate, 1)]:
        profile_name = 'bytes-'+name
        profile = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'DiagnosticProfile', 'metadata': {'name': profile_name}, 'spec': {
            'mode': 'fixed', 'budget': {'maxAdditionalChecks': 3, 'maxDuration': '60s', 'maxParallel': 1, 'maxEstimatedBytes': budget},
            'hypotheses': [{'id': 'responding'}, {'id': 'not-responding'}],
            'actions': [{'id': 'http-budget', 'check': check, 'expectedOutcomes': {'responding': 'success', 'not-responding': 'failure'}, 'placement': {'agentRefs': ['branch'], 'zones': ['office'], 'strategy': 'fixed'}}],
            'fixed': {'steps': [{'id': 'first', 'actionRef': 'http-budget'}, {'id': 'second', 'actionRef': 'http-budget', 'dependsOn': ['first']}]}}}
        created = request('POST', '/api/v1/resources/DiagnosticProfile', profile, **auth)
        assert created['status'] == 201, created
        lab.wait(lambda: request('GET', '/api/v1/auth/me', **auth)['status'] == 200 and lab.assessment().get('state') == 'healthy', 'healthy after profile creation')
        request_id = secrets.token_hex(16)
        def start():
            response = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': profile_name, 'requestId': request_id}, **auth)
            assert response['status'] in [200, 202, 409, 503], response
            return response['body'] if response['status'] in [200, 202] else None
        run = lab.wait(start, 'idempotent byte-budget manual start')['id']
        def terminal():
            response = request('GET', '/api/v1/diagnostics/'+run, **auth)
            return response['body'] if response['status'] == 200 and response['body']['state'] not in ['created', 'running'] else None
        detail = lab.wait(terminal, 'byte-budget terminal diagnostic')
        tasks = request('GET', '/api/v1/diagnostics/'+run+'/tasks', **auth)['body']['items']
        assert detail['state'] == 'inconclusive' and detail['stopReason'] == 'no_admissible_action', detail
        assert detail['budgetUsed'] == {'additionalChecks': want, 'estimatedBytes': want*estimate}, detail
        assert len(tasks) == want and all(t['estimatedBytes'] == estimate and len(t['observationIds']) == 1 for t in tasks), tasks
        cases[name] = {'run': run, 'budget': budget, 'detail': detail, 'tasks': tasks}
    lab.compose('restart', 'controller')
    for case in cases.values():
        lab.wait(lambda: request('GET', '/api/v1/diagnostics/'+case['run'], **auth)['status'] == 200, 'public detail after restart')
        restored = request('GET', '/api/v1/diagnostics/'+case['run'], **auth)['body']
        assert restored['budgetUsed'] == case['detail']['budgetUsed']
    lab.wait(lambda: lab.assessment().get('state') == 'healthy', 'healthy after restart')
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'realPostgreSQLAndAgents': True, 'trustedHTTPS': True, 'model': 'protocol-envelope-v1', 'cases': cases, 'restartPreservedBudget': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project', required=True); parser.add_argument('--prefix', required=True); parser.add_argument('--output', required=True)
    args = parser.parse_args(); path = Path(args.output)
    if path.exists(): raise FileExistsError('Use a new output path')
    result = verify(Lab(args.project, args.prefix)); path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    with path.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
    path.chmod(0o600)
    print('PASS: live zero/exact-byte admission, physical estimates and restart persistence; secrets omitted')
