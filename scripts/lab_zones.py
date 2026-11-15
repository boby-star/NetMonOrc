"""Verify explicit any-zone SLA against real failures and missing evidence."""
import copy
import json
import uuid


def verify(lab, cli, common, first, client):
    def command(*args, value=None):
        return json.loads(lab.compose('exec', '-T', 'controller', *cli, args[0],
                         '--context-file', '/tmp/cli/context.json', *args[1:],
                         input=json.dumps(value).encode() if value is not None else None))

    def get(kind, name=None):
        return command('get', kind, *([name] if name else []))

    def apply(manifests, versions, wait=True):
        return command('apply', '-f', '-', *(['--wait', '--timeout', '45s'] if wait else []),
                       value={'manifests': manifests, 'expectedVersions': versions, 'requestId': str(uuid.uuid4())})

    def revision(op):
        return next(t['effectiveRevision'] for t in op['result']['targets'] if t['kind'] == 'Monitor')

    def matches(rev, state, office, server):
        a = lab.assessment()
        zones = {z['zone']: z['state'] for z in a.get('zones', [])}
        return a.get('revision') == rev and a.get('state') == state and zones == {'office': office, 'server': server}

    def api_check(rev, state, coverage, office, server):
        proof = json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client,
                   input=json.dumps({**common, **first, 'mode': 'assessment-check', 'revision': rev,
                        'state': state, 'coverage': coverage, 'zones': {'office': office, 'server': server}}).encode()))
        assert proof['assessmentVerified']

    def agents_ready():
        return all(a['onlineState'] == 'online' and a.get('appliedRevision') and a.get('lastHeartbeat')
                   for a in get('agents')['items'])

    original = get('Monitor', 'research-web')
    policy = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'AssessmentPolicy',
              'metadata': {'name': 'zone-sla'}, 'spec': copy.deepcopy(original['manifest']['spec']['assessment'])}
    policy['spec']['zoneAggregation'] = 'any'
    monitor = copy.deepcopy(original['manifest'])
    del monitor['spec']['assessment']
    monitor['spec']['assessmentRef'] = 'zone-sla'
    monitor['spec']['diagnostics']['enabled'] = False
    op = apply([monitor, policy], {'Monitor/research-web': original['resourceVersion']})
    assert op['state'] == 'completed'
    rev = revision(op)
    lab.wait(lambda: matches(rev, 'healthy', 'healthy', 'healthy'), 'any SLA has current physical evidence from both zones')
    source = get('Monitor', 'research-web')

    lab.inject('office-path')
    lab.wait(lambda: matches(rev, 'healthy', 'down', 'healthy'), 'office path failure stays visible under any SLA')
    api_check(rev, 'healthy', 'sufficient', 'down', 'healthy')
    stored = get('AssessmentPolicy', 'zone-sla')
    all_policy = copy.deepcopy(policy)
    all_policy['spec']['zoneAggregation'] = 'all-required'
    changed = apply([all_policy], {'AssessmentPolicy/zone-sla': stored['resourceVersion']})
    assert changed['state'] == 'completed'
    all_rev = revision(changed)
    assert all_rev != rev and get('Monitor', 'research-web')['resourceVersion'] == source['resourceVersion']
    lab.wait(lambda: matches(all_rev, 'degraded', 'down', 'healthy'), 'same path failure under all-required is degraded')
    api_check(all_rev, 'degraded', 'sufficient', 'down', 'healthy')
    lab.inject('healthy')
    lab.wait(lambda: matches(all_rev, 'healthy', 'healthy', 'healthy'), 'physical path recovery restores all-required healthy')

    stored = get('AssessmentPolicy', 'zone-sla')
    changed = apply([policy], {'AssessmentPolicy/zone-sla': stored['resourceVersion']})
    assert changed['state'] == 'completed' and revision(changed) == rev
    lab.wait(lambda: matches(rev, 'healthy', 'healthy', 'healthy'), 'any restored')
    lab.compose('stop', 'branch')
    lab.wait(lambda: matches(rev, 'healthy', 'unknown', 'healthy'), 'any remains available while missing source reduces coverage')
    api_check(rev, 'healthy', 'degraded', 'unknown', 'healthy')
    before = lab.assessment()['at']
    lab.compose('restart', 'controller')
    lab.wait(lambda: lab.assessment().get('at', '') > before and matches(rev, 'healthy', 'unknown', 'healthy'), 'any policy survives restart with offline office Agent')
    lab.compose('exec', '-T', 'controller', *cli, 'login', '--server', 'https://localhost:8443',
                '--ca-file', '/run/netmonorc/ca.crt', '--context-file', '/tmp/cli/context.json',
                '--token-stdin', input=first['apiToken'].encode())
    assert get('AssessmentPolicy', 'zone-sla')['manifest']['spec']['zoneAggregation'] == 'any'

    restored = apply([original['manifest']], {'Monitor/research-web': source['resourceVersion']}, wait=False)
    restored_rev = revision(restored)
    lab.wait(lambda: matches(restored_rev, 'unknown', 'unknown', 'healthy'), 'all-required refuses healthy while required office evidence missing')
    api_check(restored_rev, 'unknown', 'degraded', 'unknown', 'healthy')
    lab.compose('start', 'branch')
    lab.wait(lambda: matches(restored_rev, 'healthy', 'healthy', 'healthy') and agents_ready(), 'reconnect restores both required zones')
    lab.wait(lambda: get('operations', restored['id'])['state'] == 'completed', 'restored apply completes only after required Agent ACK')
    deleted = json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', client,
                    input=json.dumps({**common, **first, 'mode': 'rule-delete', 'kind': 'AssessmentPolicy', 'name': 'zone-sla'}).encode()))
    assert deleted['deleted']
    return {'anySLAWithRealZonePathFailure': True, 'negativeZonePreserved': True,
            'allRequiredSameFailureIsDegraded': True, 'missingAgentAnyHealthyCoverageDegraded': True,
            'missingAgentAllRequiredUnknown': True, 'zonePolicyRevisionFenced': True,
            'zonePolicySurvivesRestart': True, 'zonePolicySourceVersionPreserved': True,
            'assessmentRESTPreservesStateAndCoverage': True, 'restoredRequiredAgentACK': True}
