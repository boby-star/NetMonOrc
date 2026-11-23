"""Typed trigger, committed Fixed extraction and shared diagnostic placement."""
import argparse
import copy
import json
from pathlib import Path
import secrets
from lab import ROOT
from lab_check_placement import PlacementLab
from lab_replicas import CLIENT


def profile(name, parallel=2, missing=False):
    check = {'type': 'http', 'runner': 'native-http', 'timeout': '5s', 'config': {'url': 'http://198.18.0.80:8080/parsed', 'expectedStatus': [200]}}
    read = {**check, 'extract': [{'name': 'depth', 'from': 'response.body', 'pattern': 'queue_depth=([0-9]+)', 'group': 1, 'as': 'integer'}, {'name': 'ready', 'from': 'response.body', 'pattern': 'ready=(true|false)', 'group': 1, 'as': 'boolean'}]}
    condition = 'tasks["a-read"].values.absent == 0' if missing else 'has(tasks["a-read"].values.depth) && tasks["a-read"].values.depth == 0 && tasks["a-read"].values.ready && tasks["a-read"].reasonCode == "expected_status"'
    return {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'DiagnosticProfile', 'metadata': {'name': name}, 'spec': {'mode': 'fixed', 'budget': {'maxAdditionalChecks': 2, 'maxDuration': '60s', 'maxParallel': parallel}, 'hypotheses': [{'id': 'responding'}, {'id': 'broken'}], 'actions': [
        {'id': 'read-body', 'check': read, 'placementRef': 'parsed-office', 'expectedOutcomes': {'responding': 'success', 'broken': 'failure'}},
        {'id': 'confirm', 'check': check, 'placement': {'selector': {'matchLabels': {'site': 'server'}}, 'strategy': 'fixed', 'distinctBy': 'failureDomain', 'replicas': 1}, 'expectedOutcomes': {'responding': 'success', 'broken': 'failure'}}],
        'fixed': {'steps': [{'id': 'a-read', 'actionRef': 'read-body'}, {'id': 'b-confirm', 'actionRef': 'confirm', 'dependsOn': ['a-read'], 'when': condition}]}}}


def verify(lab):
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project; users preserved')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'declarative-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'declarative-admin', 'password': password}); password = ''; assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    path = '/api/v1/resources/Monitor/research-web'; current = request('GET', path, **auth)
    bad = copy.deepcopy(current['body']['manifest']); bad['spec']['diagnostics']['when'] = 'monitor.sttae == "down"'
    rejected = request('PUT', path, bad, etag=current['etag'], **auth); assert rejected['status'] == 422, rejected
    assert request('GET', path, **auth)['etag'] == current['etag']
    good = copy.deepcopy(current['body']['manifest']); good['spec']['diagnostics']['when'] = 'monitor.name == "research-web" && monitor.coverageState != "unknown"'
    update = request('PUT', path, good, etag=current['etag'], **auth); assert update['status'] == 200, update
    revision = update['body']['status']['desiredRevision']
    def healthy():
        r = request('GET', '/api/v1/monitors/research-web/assessment', **auth)
        return r['status'] == 200 and r['body']['state'] == 'healthy' and r['body']['revision'] == revision
    lab.wait(healthy, 'fresh healthy typed trigger revision', seconds=60)
    policy = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'PlacementPolicy', 'metadata': {'name': 'parsed-office'}, 'spec': {'selector': {'matchLabels': {'site': 'office'}}, 'zones': ['office'], 'strategy': 'leastLoaded', 'replicas': 1}}
    response = request('POST', '/api/v1/resources/PlacementPolicy', policy, **auth); assert response['status'] == 201, response
    cli = ('exec', '-T', 'controller', '/usr/local/bin/netmonorc', 'validate', '--config', '-', '--slots', 'branch=office,dc=server', '--agent-labels-file', '/run/netmonorc/agent-labels.json')
    cli_validation = json.loads(lab.compose(*cli, input=(ROOT/'examples/diagnostics/parsed-fixed.yaml').read_bytes())); assert cli_validation['valid']
    cases = {}
    for name, parallel, missing in [('parsed-serial', 1, False), ('parsed-parallel', 2, False), ('parsed-missing', 2, True)]:
        manifest = profile(name, parallel, missing)
        response = request('POST', '/api/v1/resources/DiagnosticProfile', manifest, **auth); assert response['status'] == 201, response
        request_id = secrets.token_hex(16)
        def start():
            r = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': name, 'requestId': request_id}, **auth)
            assert r['status'] in [200, 202, 409, 503], r
            return r['body']['id'] if r['status'] in [200, 202] else None
        session_id = lab.wait(start, 'manual parsed diagnosis')
        def terminal():
            r = request('GET', '/api/v1/diagnostics/'+session_id, **auth)
            return r['body'] if r['status'] == 200 and r['body']['state'] not in ['created', 'running'] else None
        session = lab.wait(terminal, 'terminal parsed diagnosis', seconds=60)
        tasks = request('GET', '/api/v1/diagnostics/'+session_id+'/tasks?limit=200', **auth)['body']['items']
        assert session['state'] == ('inconclusive' if missing else 'completed'), session
        assert session['budgetUsed']['additionalChecks'] == (1 if missing else 2), session
        by_step = {t['stepId']: t for t in tasks}; assert by_step['a-read']['agentId'] == 'branch', tasks
        if not missing: assert by_step['b-confirm']['agentId'] == 'dc', tasks
        evidence = request('GET', '/api/v1/observations/'+by_step['a-read']['observationIds'][0], **auth)
        assert evidence['status'] == 200 and evidence['body']['values']['depth'] == 0 and evidence['body']['values']['ready'] is True, evidence
        cases[name] = {'session': session, 'tasks': tasks, 'readObservation': evidence['body']}
        lab.wait(healthy, 'regular healthy between sessions')
    policy_view = request('GET', '/api/v1/resources/PlacementPolicy/parsed-office', **auth)
    blocked = request('DELETE', '/api/v1/resources/PlacementPolicy/parsed-office', etag=policy_view['etag'], **auth); assert blocked['status'] == 409, blocked
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'unknownTriggerRejected': rejected['status'], 'triggerRejectionPreservedETag': True, 'typedTriggerRevision': revision, 'cliYAMLValidation': cli_validation, 'cases': cases, 'referencedPlacementDeletion': blocked['status'], 'regularHealthy': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('action', choices=['prepare', 'verify', 'down']); p.add_argument('--project', required=True); p.add_argument('--prefix', required=True); p.add_argument('--output')
    a = p.parse_args(); lab = PlacementLab(a.project, a.prefix)
    if a.action == 'prepare': lab.prepare_inventory()
    elif a.action == 'down': lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not a.output: p.error('--output required')
        output = Path(a.output)
        if output.exists(): raise FileExistsError('Use a new output path')
        lab.up(); result = verify(lab); output.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        with output.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
        output.chmod(0o600); print('PASS: typed trigger, committed Fixed values and selector/ref placement')
