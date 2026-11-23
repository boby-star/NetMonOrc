"""Real replicated diagnostics, complete CEL contexts, budgets and cancellation."""
import argparse
import copy
import json
from pathlib import Path
import secrets
from lab_check_placement import PlacementLab
from lab_replicas import CLIENT


def replica_profile(name, mode='fixed', parallel=2, budget=3):
    read = {'id': 'read', 'check': {'type': 'http', 'runner': 'native-http', 'timeout': '5s', 'config': {'url': 'http://198.18.0.80:8080/parallel-slow', 'expectedStatus': [200]}, 'extract': [{'name': 'depth', 'from': 'response.body', 'pattern': 'queue_depth=([0-9]+)', 'group': 1, 'as': 'integer'}]}, 'placementRef': 'both-probes', 'expectedOutcomes': {'responding': 'success', 'broken': 'failure'}}
    spec = {'mode': mode, 'budget': {'maxAdditionalChecks': budget, 'maxDuration': '90s', 'maxParallel': parallel}, 'hypotheses': [{'id': 'responding'}, {'id': 'broken'}], 'actions': [read]}
    if mode == 'adaptive': spec['adaptive'] = {}
    else:
        spec['actions'].append({'id': 'confirm', 'check': {'type': 'http', 'runner': 'native-http', 'timeout': '5s', 'config': {'url': 'http://198.18.0.80:8080/confirm', 'expectedStatus': [200]}}, 'placement': {'strategy': 'fixed', 'agentRefs': ['branch'], 'zones': ['office']}, 'expectedOutcomes': {'responding': 'success', 'broken': 'failure'}})
        expression = 'monitor.name == "research-web" && monitor.labels.environment == "research" && target.attributes.serial == 9007199254740993 && zone.name == "office" && checks.http.outcome == "success" && zones.server.checks.http.outcome == "success" && diagnostic.attempts == 2 && diagnostic.remainingChecks == 1 && diagnostic.elapsedSeconds >= 0.0 && tasks.read.replicaResults.branch.values.depth == 0 && tasks.read.replicaResults.dc.values.depth == 0'
        spec['fixed'] = {'steps': [{'id': 'read', 'actionRef': 'read'}, {'id': 'confirm', 'actionRef': 'confirm', 'dependsOn': ['read'], 'when': expression}]}
    return {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'DiagnosticProfile', 'metadata': {'name': name}, 'spec': spec}


def verify(lab, output):
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'replica-context-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'replica-context-admin', 'password': password}); password = ''; assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    def create(manifest):
        r = request('POST', '/api/v1/resources/'+manifest['kind'], manifest, **auth); assert r['status'] == 201, r
    create({'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'Target', 'metadata': {'name': 'context-target'}, 'spec': {'type': 'endpoint', 'address': '198.18.0.80', 'attributes': {'serial': 9007199254740993}}})
    create({'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'PlacementPolicy', 'metadata': {'name': 'both-probes'}, 'spec': {'strategy': 'fixed', 'agentRefs': ['branch', 'dc'], 'selector': {'matchLabels': {'role': 'probe'}}, 'replicas': 2, 'distinctBy': 'failureDomain'}})
    path = '/api/v1/resources/Monitor/research-web'
    current = request('GET', path, **auth); manifest = current['body']['manifest']
    manifest['metadata']['labels'] = {'environment': 'research'}; manifest['spec'].pop('target'); manifest['spec']['targetRef'] = 'context-target'; manifest['spec']['diagnostics']['when'] = 'false'
    r = request('PUT', path, manifest, etag=current['etag'], **auth); assert r['status'] == 200, r
    revision = r['body']['status']['desiredRevision']
    def healthy():
        r = request('GET', '/api/v1/monitors/research-web/assessment', **auth)
        return r['status'] == 200 and r['body']['revision'] == revision and r['body']['state'] == 'healthy'
    lab.wait(healthy, 'healthy exact context revision', seconds=60)
    cases = {}
    def tasks(run): return request('GET', '/api/v1/diagnostics/'+run+'/tasks?limit=200', **auth)['body']['items']
    def terminal(run):
        def done():
            r = request('GET', '/api/v1/diagnostics/'+run, **auth)
            return r['body'] if r['status'] == 200 and r['body']['state'] not in ['created', 'running'] else None
        return lab.wait(done, 'terminal replica session', seconds=60)
    def start(ref):
        lab.wait(healthy, 'healthy before manual replica run'); key = secrets.token_hex(16)
        def submit():
            r = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': ref, 'requestId': key}, **auth)
            assert r['status'] in [200, 202, 409, 503], r
            return r['body']['id'] if r['status'] in [200, 202] else None
        return lab.wait(submit, 'manual replica admission')
    def save():
        output.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        output.write_text(json.dumps({'status': 'in_progress', 'cases': cases, 'credentialsOmitted': True}, indent=2)+'\n'); output.chmod(0o600)
    for name, mode, parallel, budget in [('fixed-serial', 'fixed', 1, 3), ('fixed-parallel', 'fixed', 2, 3), ('adaptive-pair', 'adaptive', 2, 2), ('replica-budget', 'adaptive', 2, 1)]:
        profile = replica_profile(name, mode, parallel, budget); create(profile)
        run = start(name); detail = terminal(run); physical = tasks(run)
        if budget == 1:
            assert detail['state'] == 'inconclusive' and detail['budgetUsed']['additionalChecks'] == 0 and physical == [], detail
        else:
            expected = 3 if mode == 'fixed' else 2
            assert detail['state'] == 'completed' and detail['budgetUsed']['additionalChecks'] == expected and len(physical) == expected, detail
            reads = [t for t in physical if t['actionId'] == 'read']; assert len(reads) == 2 and {t['agentId'] for t in reads} == {'branch', 'dc'}, physical
            if mode == 'fixed': assert all(t['stepId'] == 'read' for t in reads)
        observations = [request('GET', '/api/v1/observations/'+oid, **auth)['body'] for t in physical for oid in t['observationIds']]
        assert len(observations) == len(physical)
        cases[name] = {'session': detail, 'tasks': physical, 'observations': observations}; save()
    slow = replica_profile('replica-cancel', 'fixed', 2, 2); slow['spec']['actions'] = slow['spec']['actions'][:1]; slow['spec']['actions'][0]['check']['timeout'] = '15s'; slow['spec']['actions'][0]['check']['config']['url'] = 'http://198.18.0.80:8080/slow'; slow['spec']['fixed']['steps'] = [{'id': 'read', 'actionRef': 'read'}]; create(slow)
    run = start('replica-cancel')
    lab.wait(lambda: len(tasks(run)) == 2 and all(t['state'] == 'accepted' for t in tasks(run)), 'two accepted replica tasks')
    r = request('POST', '/api/v1/diagnostics/'+run+'/cancel', **auth); assert r['status'] == 202, r
    detail = terminal(run); assert detail['state'] == 'cancelled' and detail['budgetUsed']['additionalChecks'] == 2, detail
    lab.wait(lambda: all(t['observationIds'] for t in tasks(run)), 'both cancelled replica results', seconds=45)
    cases['cancel'] = {'session': detail, 'tasks': tasks(run)}; save()
    auto = replica_profile('replica-auto', 'adaptive', 2, 2); create(auto)
    current = request('GET', path, **auth); manifest = current['body']['manifest']
    manifest['spec']['diagnostics'].update({'profileRef': 'replica-auto', 'when': 'monitor.labels.environment == "research" && target.attributes.serial == 9007199254740993 && !has(monitor.weightedScore)'})
    r = request('PUT', path, manifest, etag=current['etag'], **auth); assert r['status'] == 200, r
    revision = r['body']['status']['desiredRevision']; lab.wait(healthy, 'healthy before automatic trigger', seconds=60)
    try:
        lab.inject('http')
        run = lab.wait(lambda: lab.sql("SELECT id FROM nmo_diagnostic_runs WHERE profile_ref='replica-auto' ORDER BY started_at DESC LIMIT 1"), 'automatic label/attribute trigger', seconds=60)
        detail = terminal(run); physical = tasks(run)
        assert detail['state'] == 'completed' and detail['remainingHypotheses'] == ['broken'] and detail['budgetUsed']['additionalChecks'] == 2 and len(physical) == 2, detail
        cases['automatic-trigger'] = {'session': detail, 'tasks': physical}; save()
    finally:
        lab.inject('healthy')
    lab.wait(healthy, 'regular monitoring restored', seconds=60)
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'cases': cases, 'regularHealthy': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('action', choices=['prepare', 'verify', 'down']); p.add_argument('--project', required=True); p.add_argument('--prefix', required=True); p.add_argument('--output')
    a = p.parse_args(); lab = PlacementLab(a.project, a.prefix)
    if a.action == 'prepare': lab.prepare_inventory()
    elif a.action == 'down': lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not a.output: p.error('--output required')
        output = Path(a.output)
        if output.exists(): raise FileExistsError('Use a new output path')
        lab.up(); result = verify(lab, output); output.write_text(json.dumps(result, indent=2)+'\n'); output.chmod(0o600); print('PASS: physical replicas, full diagnostic CEL context, budgets and cancellation')
