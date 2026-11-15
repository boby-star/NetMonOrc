"""Declared Dependency context: ACK, graph rejection, no health inheritance."""
import json
import uuid


def verify(lab, cli, common, first, client):
    def command(*args, value=None):
        return json.loads(lab.compose('exec', '-T', 'controller', *cli, args[0], '--context-file', '/tmp/cli/context.json', *args[1:], input=json.dumps(value).encode() if value is not None else None))
    def get(kind, name):
        return command('get', kind, name)
    def apply(manifests, versions):
        return command('apply', '-f', '-', '--wait', '--timeout', '45s', value={'manifests': manifests, 'expectedVersions': versions, 'requestId': str(uuid.uuid4())})
    def revision(op):
        return next(t['effectiveRevision'] for t in op['result']['targets'] if t['kind'] == 'Monitor')
    def healthy(rev):
        return lab.assessment().get('state') == 'healthy' and lab.assessment().get('revision') == rev
    original = get('Monitor', 'research-web')
    original_rev = original['status']['desiredRevision']
    target = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'Target', 'metadata': {'name': 'context-resolver'}, 'spec': {'type': 'logical'}}
    link = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'Dependency', 'metadata': {'name': 'web-needs-resolver'}, 'spec': {'from': {'kind': 'Monitor', 'name': 'research-web'}, 'to': {'kind': 'Target', 'name': 'context-resolver'}}}
    created = apply([link, target], {})
    assert created['state'] == 'completed' and any(t['kind'] == 'Monitor' and t.get('implicit') and t['reason'] == 'required_agents_applied' for t in created['result']['targets'])
    rev = revision(created)
    assert rev != original_rev and get('Monitor', 'research-web')['resourceVersion'] == original['resourceVersion']
    lab.wait(lambda: healthy(rev), 'declared dependency context reaches Agent without changing healthy service')
    assert get('Dependency', 'web-needs-resolver')['manifest']['spec'] == link['spec']
    reverse = {'apiVersion': link['apiVersion'], 'kind': 'Dependency', 'metadata': {'name': 'resolver-needs-web'}, 'spec': {'from': link['spec']['to'], 'to': link['spec']['from']}}
    rejected = json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client, input=json.dumps({**common, **first, 'mode': 'dependency-invalid', 'manifest': reverse}).encode()))
    assert rejected['cycleRejected'] and get('Monitor', 'research-web')['status']['desiredRevision'] == rev
    guarded = json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client, input=json.dumps({**common, **first, 'mode': 'dependency-delete-guard'}).encode()))
    assert guarded['referenceProtected']
    current = get('Dependency', 'web-needs-resolver')
    link['spec']['strength'] = 'hard'
    changed = apply([link], {'Dependency/web-needs-resolver': current['resourceVersion']})
    hard_rev = revision(changed)
    assert changed['state'] == 'completed' and hard_rev != rev
    assert get('Monitor', 'research-web')['resourceVersion'] == original['resourceVersion']
    lab.wait(lambda: healthy(hard_rev), 'hard dependency does not invent failure of logical Target')
    assert lab.sql("SELECT count(*) FROM nmo_observations WHERE monitor<>'research-web'") == '0'
    before = lab.assessment()['at']
    lab.compose('restart', 'controller')
    lab.wait(lambda: lab.assessment().get('at', '') > before and healthy(hard_rev), 'persisted dependency context survives Controller restart')
    lab.compose('exec', '-T', 'controller', *cli, 'login', '--server', 'https://localhost:8443', '--ca-file', '/run/netmonorc/ca.crt', '--context-file', '/tmp/cli/context.json', '--token-stdin', input=first['apiToken'].encode())
    assert get('Dependency', 'web-needs-resolver')['manifest']['spec']['strength'] == 'hard'
    for kind, name in [('Dependency', 'web-needs-resolver'), ('Target', 'context-resolver')]:
        deleted = json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client, input=json.dumps({**common, **first, 'mode': 'rule-delete', 'kind': kind, 'name': name}).encode()))
        assert deleted['deleted']
    def restored():
        agents = command('get', 'agents')['items']
        return healthy(original_rev) and len(agents) == 2 and all(a['onlineState'] == 'online' and a.get('lastHeartbeat') and a.get('appliedRevision') == original_rev for a in agents)
    lab.wait(restored, 'dependency removal restores original revision and Agent proof')
    return {'dependencyCRUDAndMixedApply': True, 'dependencyCLIAndAgentACK': True, 'requiresCycleRejectedAtomically': True, 'dependencyReferenceDeleteProtected': True, 'dependencyChangeFencesRevision': True, 'monitorSourceVersionPreserved': True, 'hardDependencyDoesNotPropagateFailure': True, 'dependencyContextSurvivesRestart': True, 'dependencyCreatesNoExtraProbes': True, 'dependencyRemovalRestoresRevision': True}
