"""Real Fixed DAG overlap, stable step provenance, quotas and cancellation."""
import argparse
import copy
from datetime import datetime
import json
from pathlib import Path
import secrets
from lab import Lab
from lab_replicas import CLIENT


def profile(name, checks=3, byte_budget=None, slow=False):
    actions = []
    for action, agent, path in [('left', 'branch', '/parallel-slow'), ('right', 'dc', '/parallel-fast'), ('join', 'branch', '/parallel-join')]:
        actions.append({'id': action, 'check': {'type': 'http', 'runner': 'native-http', 'timeout': '15s' if slow else '5s', 'config': {'url': 'http://198.18.0.80:8080'+('/slow' if slow and action != 'join' else path), 'expectedStatus': [200]}},
                        'placement': {'agentRefs': [agent], 'strategy': 'fixed'}, 'expectedOutcomes': {'responding': 'success', 'broken': 'failure'}})
    budget = {'maxAdditionalChecks': checks, 'maxDuration': '60s', 'maxParallel': 2}
    if byte_budget is not None: budget['maxEstimatedBytes'] = byte_budget
    return {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'DiagnosticProfile', 'metadata': {'name': name}, 'spec': {
        'mode': 'fixed', 'budget': budget, 'hypotheses': [{'id': 'responding'}, {'id': 'broken'}], 'actions': actions,
        'fixed': {'steps': [{'id': 'a-join', 'actionRef': 'join', 'dependsOn': ['b-left', 'c-right'], 'when': 'tasks["b-left"].outcome == "success" && tasks["c-right"].outcome == "success"'}, {'id': 'c-right', 'actionRef': 'right'}, {'id': 'b-left', 'actionRef': 'left'}]}}}


def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project; preserve existing users')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'parallel-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'parallel-admin', 'password': password}); password = ''
    assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    original = request('GET', '/api/v1/resources/Monitor/research-web', **auth)
    estimate = 32768+16384+len('http://198.18.0.80:8080/parallel-slow')
    manifests = [profile('parallel-full'), profile('parallel-count', checks=1), profile('parallel-bytes', byte_budget=estimate), profile('parallel-cancel', slow=True)]
    for manifest in manifests:
        response = request('POST', '/api/v1/resources/DiagnosticProfile', manifest, **auth)
        assert response['status'] == 201, response
        lab.wait(lambda: request('GET', '/api/v1/auth/me', **auth)['status'] == 200, 'profile catalog API')
    def healthy():
        response = request('GET', '/api/v1/monitors/research-web/assessment', **auth)
        return response['status'] == 200 and response['body']['state'] == 'healthy'
    def start(ref):
        lab.wait(healthy, 'healthy regular coverage before manual run')
        request_id = secrets.token_hex(16)
        def submit():
            response = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': ref, 'requestId': request_id}, **auth)
            assert response['status'] in [200, 202, 409, 503], response
            return response['body'] if response['status'] in [200, 202] else None
        run = lab.wait(submit, 'idempotent parallel start')['id']
        assert len(run) == 32 and all(c in '0123456789abcdef' for c in run)
        return run
    def terminal(run):
        def done():
            response = request('GET', '/api/v1/diagnostics/'+run, **auth)
            return response['body'] if response['status'] == 200 and response['body']['state'] not in ['created', 'running'] else None
        return lab.wait(done, 'terminal parallel run', seconds=60)
    def tasks(run):
        response = request('GET', '/api/v1/diagnostics/'+run+'/tasks', **auth)
        assert response['status'] == 200, response
        return response['body']['items']
    def observations(run):
        return json.loads(lab.sql("SELECT COALESCE(json_agg(json_build_object('stepId',t.step_id,'observation',o.payload) ORDER BY t.step_id),'[]')::text FROM nmo_diagnostic_tasks t JOIN nmo_observations o ON o.diagnostic_task_id=t.id WHERE t.run_id='"+run+"'"))
    def instant(value): return datetime.fromisoformat(value.replace('Z', '+00:00'))
    cases = {}; trace = None
    for repetition in [1, 2]:
        run = start('parallel-full'); detail = terminal(run); physical = tasks(run); evidence = observations(run)
        assert detail['state'] == 'completed' and detail['remainingHypotheses'] == ['responding'] and detail['budgetUsed']['additionalChecks'] == 3, detail
        journal = detail['explanation']['decisions']; assert [d['stepId'] for d in journal] == ['b-left', 'c-right', 'a-join'], journal
        current = [(d['stepId'], d['actionId'], d['agentId'], d['attemptNo'], d['hypothesesBefore']) for d in journal]
        if trace is None: trace = copy.deepcopy(current)
        else: assert current == trace, 'identical Fixed inputs changed logical journal'
        by_step = {e['stepId']: e['observation'] for e in evidence}
        assert len(physical) == len(by_step) == 3 and all(len(t['observationIds']) == 1 and t['stepId'] in by_step for t in physical)
        left, right, join = [by_step[key] for key in ['b-left', 'c-right', 'a-join']]
        assert max(instant(left['startedAt']), instant(right['startedAt'])) < min(instant(left['finishedAt']), instant(right['finishedAt'])), 'independent probes did not overlap'
        assert instant(right['finishedAt']) < instant(left['finishedAt']), 'controlled fast/slow completion order not observed'
        assert instant(join['startedAt']) >= max(instant(left['finishedAt']), instant(right['finishedAt'])), 'join started before parents finished'
        assert lab.sql("SELECT max_parallel FROM nmo_diagnostic_runs WHERE id='"+run+"'") == '2'
        cases['overlap-'+str(repetition)] = {'detail': detail, 'tasks': physical, 'evidence': evidence, 'parentsOverlap': True, 'joinAfterBoth': True, 'journalIndependentOfFinishOrder': True}
    for name in ['count', 'bytes']:
        run = start('parallel-'+name); detail = terminal(run); physical = tasks(run)
        assert detail['state'] == 'inconclusive' and detail['budgetUsed']['additionalChecks'] == 1 and len(physical) == 1 and physical[0]['stepId'] == 'b-left', detail
        if name == 'bytes': assert detail['budgetUsed']['estimatedBytes'] == estimate
        cases[name] = {'detail': detail, 'tasks': physical}
    run = start('parallel-cancel')
    lab.wait(lambda: len(tasks(run)) == 2 and all(t['state'] == 'accepted' for t in tasks(run)), 'two accepted slow parallel tasks')
    cancelled = request('POST', '/api/v1/diagnostics/'+run+'/cancel', **auth)
    assert cancelled['status'] == 202, cancelled
    detail = terminal(run); assert detail['state'] == 'cancelled' and detail['budgetUsed']['additionalChecks'] == 2, detail
    lab.wait(lambda: len(observations(run)) == 2, 'both cancelled physical workers report')
    physical = tasks(run); assert len(physical) == 2 and all(t['stepId'] in ['b-left', 'c-right'] for t in physical)
    assert all(e['observation']['execution'] == 'execution_failed' and e['observation']['outcome'] == 'unknown' for e in observations(run))
    cases['cancelledWave'] = {'detail': detail, 'tasks': physical, 'evidence': observations(run), 'joinNotDispatched': True}
    lab.wait(healthy, 'regular monitoring remains healthy')
    final = request('GET', '/api/v1/resources/Monitor/research-web', **auth)
    assert final['etag'] == original['etag'] and final['body']['status']['desiredRevision'] == original['body']['status']['desiredRevision']
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'realPostgreSQLAndMTLSAgents': True, 'trustedHTTPS': True, 'cases': cases, 'regularHealthy': True, 'monitorSourceUnchanged': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project', required=True); parser.add_argument('--prefix', required=True); parser.add_argument('--output', required=True)
    args = parser.parse_args(); path = Path(args.output)
    if path.exists(): raise FileExistsError('Use a new output path')
    result = verify(Lab(args.project, args.prefix)); path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    with path.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
    path.chmod(0o600); print('PASS: real parallel Fixed waves, stable step provenance, quotas and in-flight cancellation; secrets omitted')
