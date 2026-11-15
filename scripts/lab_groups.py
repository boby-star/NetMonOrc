"""Critical ResourceGroup integration; groups never create physical probes."""
import copy
import json
import uuid


def verify(lab, cli, common, first, client):
    def command(*args, value=None):
        return json.loads(lab.compose('exec', '-T', 'controller', *cli, args[0], '--context-file', '/tmp/cli/context.json', *args[1:], input=json.dumps(value).encode() if value is not None else None))
    def get(kind, name):
        return command('get', kind, name)
    def apply(manifests, versions, wait=True):
        return command('apply', '-f', '-', *(['--wait', '--timeout', '45s'] if wait else []), value={'manifests': manifests, 'expectedVersions': versions, 'requestId': str(uuid.uuid4())})
    def read(name):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client, input=json.dumps({**common, **first, 'mode': 'group-read', 'name': name}).encode()))
    original = get('Monitor', 'research-web')
    monitor = copy.deepcopy(original['manifest'])
    monitor['metadata']['labels'] = {**monitor['metadata'].get('labels', {}), 'group': 'demo'}
    standby = copy.deepcopy(monitor)
    standby['metadata']['name'] = 'group-standby'
    standby['spec']['diagnostics']['enabled'] = False
    op = apply([monitor, standby], {'Monitor/research-web': original['resourceVersion']}, wait=False)
    lab.wait(lambda: lab.assessment().get('state') == 'healthy', 'active Monitor remains healthy')
    active = get('Monitor', 'research-web')
    def group(name, spec):
        return {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'ResourceGroup', 'metadata': {'name': name}, 'spec': spec}
    groups = [group('group-all', {'members': ['research-web', 'group-standby'], 'mode': 'derived', 'evaluation': {'strategy': 'all-required'}}),
              group('group-weighted', {'members': ['research-web', 'group-standby'], 'mode': 'derived', 'evaluation': {'strategy': 'weighted', 'weights': {'research-web': .95, 'group-standby': .05}, 'minCoverage': .95}}),
              group('group-selector', {'selector': {'matchLabels': {'group': 'demo'}}, 'mode': 'derived', 'evaluation': {'strategy': 'any'}}),
              group('group-context', {'members': []})]
    created = apply(groups, {})
    assert created['state'] == 'completed' and all(t['reason'] == 'group_catalogued' for t in created['result']['targets'])
    all_view = read('group-all')
    assert all_view['state'] == 'unknown' and all_view['coverageState'] == 'degraded'
    assert {m['monitorId']: m['state'] for m in all_view['members']} == {'research-web': 'healthy', 'group-standby': 'unknown'}
    weighted = read('group-weighted')
    assert weighted['state'] == 'healthy' and weighted['coverageState'] == 'degraded'
    cli_weighted=get('groups','group-weighted')
    assert cli_weighted['state']==weighted['state'] and cli_weighted['coverageState']==weighted['coverageState']
    assert read('group-context')['state'] is None and read('group-context')['mode'] == 'context'
    assert read('group-selector')['state'] == 'unknown'
    selector_before = get('ResourceGroup', 'group-selector')
    standby_view = get('Monitor', 'group-standby')
    standby['metadata']['labels'] = {'group': 'outside'}
    changed = apply([standby], {'Monitor/group-standby': standby_view['resourceVersion']}, wait=False)
    assert any(t['kind'] == 'ResourceGroup' and t['name'] == 'group-selector' and t.get('implicit') for t in changed['result']['targets'])
    lab.wait(lambda: read('group-selector')['state'] == 'healthy', 'selector membership reconciled from Monitor labels')
    selector_after = get('ResourceGroup', 'group-selector')
    assert selector_after['resourceVersion'] == selector_before['resourceVersion'] and selector_after['status']['desiredRevision'] != selector_before['status']['desiredRevision']
    assert lab.sql("SELECT count(*) FROM nmo_observations WHERE monitor IN ('group-all','group-weighted','group-context','group-selector','group-standby')") == '0'
    previous = lab.assessment()['at']
    lab.compose('restart', 'controller')
    lab.wait(lambda: lab.assessment().get('at', '') > previous and lab.assessment().get('state') == 'healthy', 'group catalog survives restart')
    lab.compose('exec', '-T', 'controller', *cli, 'login', '--server', 'https://localhost:8443', '--ca-file', '/run/netmonorc/ca.crt', '--context-file', '/tmp/cli/context.json', '--token-stdin', input=first['apiToken'].encode())
    assert read('group-selector')['state'] == 'healthy' and read('group-all')['state'] == 'unknown'
    for manifest in groups:
        deleted = json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client, input=json.dumps({**common, **first, 'mode': 'rule-delete', 'kind': 'ResourceGroup', 'name': manifest['metadata']['name']}).encode()))
        assert deleted['deleted']
    deleted = json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client, input=json.dumps({**common, **first, 'mode': 'rule-delete', 'kind': 'Monitor', 'name': 'group-standby'}).encode()))
    assert deleted['deleted']
    current = get('Monitor', 'research-web')
    restored = apply([original['manifest']], {'Monitor/research-web': current['resourceVersion']})
    assert restored['state'] == 'completed'
    def agents_ready():
        agents = command('get', 'agents')['items']
        return len(agents) == 2 and all(a['onlineState'] == 'online' and a.get('lastHeartbeat') and a.get('appliedRevision') for a in agents)
    lab.wait(agents_ready, 'Agent proof restored before credential revoke')
    return {'groupCRUDAndMixedApply': True, 'groupCLIAndREST': True, 'unknownMemberNotHealthyOrDown': True, 'weightedUnknownExcludedWithDegradedCoverage': True, 'contextGroupHasNoHealth': True, 'selectorMembershipAndImplicitRevision': True, 'groupSourceVersionPreserved': True, 'groupsDoNotCreateProbes': True, 'groupCatalogSurvivesRestart': True, 'standbyMonitorStillNotExecuted': True}
