"""Typed Fixed CEL guards and isolated predecessor contexts on a private stand."""
import argparse
import copy
import json
from pathlib import Path
import secrets
import subprocess
from lab import Lab, ROOT
from lab_fixed_parallel import profile
from lab_replicas import CLIENT


def verify(lab):
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project; users preserved')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'fixed-context-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'fixed-context-admin', 'password': password}); password = ''
    assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    monitor_path = '/api/v1/resources/Monitor/research-web'
    original = request('GET', monitor_path, **auth)
    good = profile('fixed-context-live', checks=4)
    good['spec']['fixed']['steps'].append({'id': 'd-finish', 'actionRef': 'join', 'dependsOn': ['a-join'], 'when': 'tasks["b-left"].executionStatus == "completed" && tasks["a-join"].outcome == "success"'})
    response = request('POST', '/api/v1/resources/DiagnosticProfile', good, **auth)
    assert response['status'] == 201, response
    path = '/api/v1/resources/DiagnosticProfile/fixed-context-live'
    saved = request('GET', path, **auth); rejections = {}
    for name, expression in {'field': 'tasks["b-left"].outcomme == "success"', 'monitor-field': 'monitor.sttae == "down"', 'unknown-step': 'tasks["missing"].outcome == "success"', 'self-reference': 'tasks["a-join"].outcome == "success"'}.items():
        bad = copy.deepcopy(good); bad['spec']['fixed']['steps'][0]['when'] = expression
        response = request('PUT', path, bad, etag=saved['etag'], **auth)
        assert response['status'] == 422, response
        rejections[name] = response['status']
    bad = copy.deepcopy(good); bad['spec']['fixed']['steps'][0]['dependsOn'] = ['b-left']
    batch = {'manifests': [bad], 'requestId': secrets.token_hex(16), 'expectedVersions': {'DiagnosticProfile/fixed-context-live': saved['body']['resourceVersion']}}
    validation = request('POST', '/api/v1/config/validate', batch, **auth)
    assert validation['status'] == 200 and not validation['body']['valid'] and 'dependsOn predecessor' in json.dumps(validation['body']), validation
    for action in ['plan', 'apply']:
        response = request('POST', '/api/v1/config/'+action, batch, **auth)
        assert response['status'] == 422, response
        rejections['undeclared-'+action] = response['status']
    assert request('GET', path, **auth)['etag'] == saved['etag']
    cli = ('exec', '-T', 'controller', '/usr/local/bin/netmonorc', 'validate', '--config', '-', '--slots', 'branch=office,dc=server')
    example = (ROOT/'examples/diagnostics/fixed-context.yaml').read_bytes()
    cli_valid = json.loads(lab.compose(*cli, input=example)); assert cli_valid['valid']
    try:
        lab.compose(*cli, input=example.replace(b'.executionStatus', b'.executionSttaus'))
        raise AssertionError('CLI admitted unknown task field')
    except subprocess.CalledProcessError as error:
        assert b'executionSttaus' in error.stderr, error.stderr
    def healthy():
        r = request('GET', '/api/v1/monitors/research-web/assessment', **auth)
        return r['status'] == 200 and r['body']['state'] == 'healthy'
    def run(ref):
        lab.wait(healthy, 'healthy before manual diagnosis'); key = secrets.token_hex(16)
        def start():
            r = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': ref, 'requestId': key}, **auth)
            assert r['status'] in [200, 202, 409, 503], r
            return r['body']['id'] if r['status'] in [200, 202] else None
        run_id = lab.wait(start, 'manual fixed start')
        def terminal():
            r = request('GET', '/api/v1/diagnostics/'+run_id, **auth)
            return r['body'] if r['status'] == 200 and r['body']['state'] not in ['created', 'running'] else None
        session = lab.wait(terminal, 'terminal fixed session', seconds=60)
        tasks = request('GET', '/api/v1/diagnostics/'+run_id+'/tasks?limit=200', **auth)['body']['items']
        return {'session': session, 'tasks': tasks}
    transitive = run('fixed-context-live')
    assert transitive['session']['state'] == 'completed' and transitive['session']['budgetUsed']['additionalChecks'] == 4, transitive
    assert {t['stepId'] for t in transitive['tasks']} == {'b-left', 'c-right', 'a-join', 'd-finish'}
    isolation = {}
    for parallel in [1, 2]:
        isolated = profile('fixed-isolated-'+str(parallel), checks=2); isolated['spec']['budget']['maxParallel'] = parallel
        isolated['spec']['fixed']['steps'] = [{'id': 'a-root', 'actionRef': 'left'}, {'id': 'b-reader', 'actionRef': 'right', 'when': 'tasks["a-" + "root"].outcome == "success"'}]
        response = request('POST', '/api/v1/resources/DiagnosticProfile', isolated, **auth); assert response['status'] == 201, response
        result = run(isolated['metadata']['name'])
        assert result['session']['state'] == 'inconclusive' and result['session']['budgetUsed']['additionalChecks'] == 1, result
        assert len(result['tasks']) == 1 and result['tasks'][0]['stepId'] == 'a-root', result
        isolation[str(parallel)] = result
    lab.wait(healthy, 'regular monitoring remains healthy')
    assert request('GET', monitor_path, **auth)['etag'] == original['etag']
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'rejections': rejections, 'validation': validation['body'], 'profileETagPreserved': True, 'cliYAMLValid': cli_valid, 'cliUnknownFieldRejected': True, 'transitive': transitive, 'isolatedComputedKey': isolation, 'monitorSourceUnchanged': True, 'regularHealthy': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('action', choices=['prepare', 'verify', 'down']); p.add_argument('--project', required=True); p.add_argument('--prefix', required=True); p.add_argument('--output')
    a = p.parse_args(); lab = Lab(a.project, a.prefix)
    if a.action == 'prepare':
        if lab.work.exists(): raise RuntimeError('Use a new owned project')
        lab.prepare()
    elif a.action == 'down':
        lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not a.output: p.error('--output required')
        output = Path(a.output)
        if output.exists(): raise FileExistsError('Use a new output path')
        lab.up(); result = verify(lab); output.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        with output.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
        output.chmod(0o600); print('PASS: typed guards, transitive predecessors and isolated serial/parallel context')
