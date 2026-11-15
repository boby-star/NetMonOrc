"""Critical fixed regular placement workflow on an already authenticated lab."""
import copy
import json
import subprocess
import uuid


def verify(lab, cli, common, first, client):
    def command(*args, value=None):
        return json.loads(lab.compose('exec', '-T', 'controller', *cli, *args,
                                     '--context-file', '/tmp/cli/context.json',
                                     input=json.dumps(value).encode() if value is not None else None))

    def get(kind, name=None):
        return command('get', kind, *([name] if name else []))

    def apply(manifests, versions, wait=True):
        batch = {'manifests': manifests, 'expectedVersions': versions, 'requestId': str(uuid.uuid4())}
        return command('apply', '-f', '-', *(['--wait', '--timeout', '45s'] if wait else []), value=batch)

    def revision(op):
        return next(t['effectiveRevision'] for t in op['result']['targets'] if t['kind'] == 'Monitor')

    def healthy(rev):
        a = lab.assessment()
        return a.get('revision') == rev and a.get('state') == 'healthy'

    def assigned(rev, expected):
        agents = get('agents')['items']
        return (len(agents) == 2 and all(a['onlineState'] == 'online' and a.get('lastHeartbeat')
                and a.get('appliedRevision') == rev and a.get('regularMonitoringAssigned') == expected[a['agentId']]
                for a in agents))

    before = get('Monitor', 'research-web')
    policy = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'PlacementPolicy',
              'metadata': {'name': 'regular-fixed'}, 'spec': {
                  'strategy': 'fixed', 'agentRefs': ['dc', 'branch'], 'zones': ['office'],
                  'replicas': 1, 'distinctBy': 'zone', 'onInsufficientAgents': 'pending'}}
    monitor = copy.deepcopy(before['manifest'])
    monitor['spec']['placementRef'] = 'regular-fixed'
    monitor['spec']['assessment']['requiredZones'] = ['office']
    batch = {'manifests': [monitor, policy],
             'expectedVersions': {'Monitor/research-web': before['resourceVersion']}, 'requestId': str(uuid.uuid4())}
    preview = command('plan', '-f', '-', value=batch)['placements']
    assert len(preview) == 1 and not preview[0]['pending']
    assert preview[0]['selectedAgents'] == [{'agentId': 'branch', 'networkZone': 'office'}]
    op = command('apply', '-f', '-', '--wait', '--timeout', '45s', value=batch)
    assert op['state'] == 'completed'
    rev = revision(op)
    lab.wait(lambda: healthy(rev) and assigned(rev, {'branch': True, 'dc': False}), 'selected source healthy; idle Agent online')
    source = get('Monitor', 'research-web')
    assert int(lab.sql("SELECT count(*) FROM nmo_observations WHERE revision='"+rev+"' AND agent_id='branch' AND diagnostic_task_id=''")) > 0
    assert lab.sql("SELECT count(*) FROM nmo_observations WHERE revision='"+rev+"' AND agent_id='dc' AND diagnostic_task_id=''") == '0'

    # A separately authorized diagnostic remains possible on the regular-idle Agent.
    profile = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'DiagnosticProfile',
               'metadata': {'name': 'idle-agent-proof'}, 'spec': {
                   'mode': 'fixed', 'budget': {'maxAdditionalChecks': 1, 'maxDuration': '20s', 'maxParallel': 1},
                   'actions': [{'id': 'idle-http', 'check': {'type': 'http', 'runner': 'native-http',
                       'timeout': '5s', 'config': {'url': 'http://198.18.0.80:8080/', 'expectedStatus': [200]}},
                       'placement': {'strategy': 'fixed', 'agentRefs': ['dc'], 'zones': ['server']}}],
                   'fixed': {'steps': [{'id': 'idle-step', 'actionRef': 'idle-http'}]}}}
    created = apply([profile], {})
    assert created['state'] == 'completed'
    admitted = None
    request_id = str(uuid.uuid4())
    def start():
        nonlocal admitted
        try:
            admitted = command('diagnostics', 'run', '--profile', 'idle-agent-proof', '--request-id', request_id, 'research-web')
            return True
        except subprocess.CalledProcessError:
            return False
    lab.wait(start, 'idle Agent diagnostic admitted after cooldown')
    run = admitted['id']
    lab.wait(lambda: lab.sql("SELECT count(*) FROM nmo_observations o JOIN nmo_diagnostic_tasks t ON t.id=o.diagnostic_task_id WHERE t.run_id='"+run+"' AND o.agent_id='dc' AND o.payload->>'outcome'='success'") == '1', 'physical HTTP diagnostic on idle Agent')
    lab.wait(lambda: get('diagnostics', run)['state'] == 'inconclusive', 'fixed DAG without hypotheses is inconclusive')

    lab.compose('stop', 'branch')
    lab.wait(lambda: lab.assessment().get('revision') == rev and lab.assessment().get('state') == 'unknown',
             'loss of selected Agent expires coverage without declaring service down')
    assert get('agents', 'dc')['onlineState'] == 'online'
    lab.compose('start', 'branch')
    lab.wait(lambda: healthy(rev) and assigned(rev, {'branch': True, 'dc': False}), 'selected Agent reconnect restores coverage')

    current = get('PlacementPolicy', 'regular-fixed')
    pending = copy.deepcopy(policy)
    pending['spec']['replicas'] = 2
    waiting = apply([pending], {'PlacementPolicy/regular-fixed': current['resourceVersion']}, wait=False)
    pending_rev = revision(waiting)
    assert pending_rev != rev and get('Monitor', 'research-web')['resourceVersion'] == source['resourceVersion']
    lab.wait(lambda: lab.assessment().get('revision') == pending_rev and lab.assessment().get('state') == 'unknown'
             and assigned(pending_rev, {'branch': False, 'dc': False}), 'insufficient fixed placement authorizes no partial probes')
    # Two full regular intervals, measured by Controller time rather than a blind sleep.
    from datetime import datetime
    start_at = datetime.fromisoformat(lab.assessment()['at'].replace('Z', '+00:00'))
    lab.wait(lambda: (datetime.fromisoformat(lab.assessment()['at'].replace('Z', '+00:00'))-start_at).total_seconds() > 12,
             'pending placement remains idle across regular intervals')
    assert lab.sql("SELECT count(*) FROM nmo_observations WHERE revision='"+pending_rev+"' AND diagnostic_task_id=''") == '0'
    waiting = get('operations', waiting['id'])
    assert waiting['state'] != 'completed'
    assert any(t['kind'] == 'Monitor' and t['reason'] == 'insufficient_placement_agents' for t in waiting['result']['targets'])
    current = get('PlacementPolicy', 'regular-fixed')
    restored = apply([policy], {'PlacementPolicy/regular-fixed': current['resourceVersion']})
    assert restored['state'] == 'completed' and revision(restored) == rev
    lab.wait(lambda: healthy(rev), 'fixed selection restored')

    previous = lab.assessment()['at']
    lab.compose('restart', 'controller')
    lab.wait(lambda: lab.assessment().get('at', '') > previous and healthy(rev), 'fixed policy survives Controller restart')
    lab.compose('exec', '-T', 'controller', *cli, 'login', '--server', 'https://localhost:8443',
                '--ca-file', '/run/netmonorc/ca.crt', '--context-file', '/tmp/cli/context.json',
                '--token-stdin', input=first['apiToken'].encode())
    lab.wait(lambda: assigned(rev, {'branch': True, 'dc': False}), 'idle and selected Agents rejoin persisted plan')
    assert get('Monitor', 'research-web')['manifest']['spec']['placementRef'] == 'regular-fixed'
    unlinked = apply([before['manifest']], {'Monitor/research-web': source['resourceVersion']})
    assert unlinked['state'] == 'completed'
    original_rev = revision(unlinked)
    for kind, name in [('PlacementPolicy', 'regular-fixed'), ('DiagnosticProfile', 'idle-agent-proof')]:
        deleted = json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client,
                             input=json.dumps({**common, **first, 'mode': 'rule-delete', 'kind': kind, 'name': name}).encode()))
        assert deleted['deleted']
    lab.wait(lambda: healthy(original_rev) and assigned(original_rev, {'branch': True, 'dc': True}), 'legacy monitoring restored after placement unlink')
    return {'fixedPlacementRESTAndCLI': True, 'typedPlacementPreview': True, 'idleAgentDoesNotRunRegularProbes': True,
            'idleAgentRunsAuthorizedDiagnostic': True, 'selectedAgentLossIsUnknown': True,
            'pendingPlacementNoPartialProbes': True, 'placementSurvivesRestart': True,
            'placementDependencyPreservesManifestVersion': True}
