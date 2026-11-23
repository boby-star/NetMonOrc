"""Real zone alerts: selector membership, scoped evidence, ACK and webhook."""
import argparse
import copy
import json
from pathlib import Path
import secrets
import time
import uuid
from lab_check_placement import PlacementLab
from lab_replicas import CLIENT


def verify(lab, output):
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'zone-alert-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'zone-alert-admin', 'password': password}); password = ''; assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    def create(m):
        r = request('POST', '/api/v1/resources/'+m['kind'], m, **auth); assert r['status'] == 201, r
        if m['kind'] == 'Monitor': time.sleep(2); request('GET', '/api/v1/auth/me', **auth)
    def resource(kind, name):
        r = request('GET', '/api/v1/resources/'+kind+'/'+name, **auth); assert r['status'] == 200, r
        return r
    def put(kind, name, manifest):
        current = resource(kind, name); r = request('PUT', '/api/v1/resources/'+kind+'/'+name, manifest, etag=current['etag'], **auth); assert r['status'] == 200, r
        if kind == 'Monitor': time.sleep(2); request('GET', '/api/v1/auth/me', **auth)
        return r
    target = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'Target', 'metadata': {'name': 'zone-target'}, 'spec': {'type': 'endpoint', 'address': '198.18.0.80', 'attributes': {'serial': 9007199254740993}}}; create(target)
    current = resource('Monitor', 'research-web'); monitor = current['body']['manifest']
    monitor['metadata']['labels'] = {'environment': 'research'}; monitor['spec'].pop('target'); monitor['spec']['targetRef'] = 'zone-target'
    monitor['spec']['checks'] = [{'id': 'http', 'type': 'http', 'runner': 'native-http', 'interval': '10s', 'timeout': '2s', 'config': {'url': 'http://198.18.0.80:8080/', 'expectedStatus': [200]}}]
    monitor['spec']['assessment']['checks'] = [{'id': 'http', 'weight': 1, 'required': True}]; monitor['spec']['diagnostics'] = {'enabled': False}
    put('Monitor', 'research-web', monitor)
    other = copy.deepcopy(monitor); other['metadata'] = {'name': 'zone-other', 'labels': {'environment': 'other'}}; create(other)
    def healthy():
        return all(request('GET', '/api/v1/monitors/'+name+'/assessment', **auth)['body'].get('state') == 'healthy' for name in ['research-web', 'zone-other'])
    lab.wait(healthy, 'healthy real observations from both Monitors', seconds=60)
    rule = {'apiVersion': 'netmonorc.io/v1alpha1', 'kind': 'AlertRule', 'metadata': {'name': 'selected-zone-outage'}, 'spec': {'selector': {'matchLabels': {'environment': 'research'}}, 'scope': 'zone', 'when': 'zone.state == "down" && checks.http.outcome == "failure" && monitor.labels.environment != "" && target.attributes.serial == 9007199254740993', 'for': '2s', 'cooldown': '0s', 'severity': 'critical', 'channelRef': 'default-webhook'}}
    applied = request('POST', '/api/v1/config/apply', {'manifests': [rule], 'requestId': str(uuid.uuid4())}, **auth); assert applied['status'] == 202, applied
    operation = lab.wait(lambda: (r if (r := request('GET', '/api/v1/operations/'+applied['body']['id'], **auth))['body']['state'] == 'completed' else None), 'zone selector apply completion', seconds=60)
    assert operation['body']['result']['targets'][0]['reason'] == 'alert_rule_evaluated'
    empty = copy.deepcopy(rule); empty['metadata']['name'] = 'empty-selector'; empty['spec']['selector'] = {'matchLabels': {'environment': 'absent'}}
    applied_empty = request('POST', '/api/v1/config/apply', {'manifests': [empty], 'requestId': str(uuid.uuid4())}, **auth); assert applied_empty['status'] == 202, applied_empty
    empty_operation = lab.wait(lambda: (r if (r := request('GET', '/api/v1/operations/'+applied_empty['body']['id'], **auth))['body']['state'] == 'completed' else None), 'empty selector catalogued', seconds=60)
    assert empty_operation['body']['result']['targets'][0]['reason'] == 'alert_selector_empty'
    current_empty = resource('AlertRule', 'empty-selector'); assert request('DELETE', '/api/v1/resources/AlertRule/empty-selector', etag=current_empty['etag'], **auth)['status'] == 204
    bad = copy.deepcopy(rule); bad['metadata']['name'] = 'bad-zone-field'; bad['spec']['when'] = 'zone.sttae == "down"'
    r = request('POST', '/api/v1/resources/AlertRule', bad, **auth); assert r['status'] == 422, r
    def alerts(name):
        r = request('GET', '/api/v1/alerts?monitor='+name+'&limit=200', **auth); assert r['status'] == 200, r
        return [a for a in r['body']['items'] if a['ruleRef'] == 'selected-zone-outage']
    def state(name, zone, wanted):
        return next((a for a in alerts(name) if a['zone'] == zone and a['state'] == wanted), None)
    initial = lab.wait(lambda: alerts('research-web') if len(alerts('research-web')) == 2 else None, 'two independent zone instances')
    assert {a['zone'] for a in initial} == {'office', 'server'} and len({a['id'] for a in initial}) == 2
    assert alerts('zone-other') == [], 'selector created unmatched instances'
    cases = {'initial': initial}
    def save():
        output.parent.mkdir(parents=True, exist_ok=True, mode=0o700); output.write_text(json.dumps({'status': 'in_progress', 'cases': cases, 'credentialsOmitted': True}, indent=2)+'\n'); output.chmod(0o600)
    save()
    try:
        lab.inject('office-path')
        office = lab.wait(lambda: state('research-web', 'office', 'firing'), 'office-only firing', seconds=60)
        assert state('research-web', 'server', 'normal'), alerts('research-web')
        evidence = [request('GET', '/api/v1/observations/'+oid, **auth)['body'] for oid in office['evidenceIds']]
        assert evidence and all(o['networkZone'] == 'office' for o in evidence), evidence
        current = request('GET', '/api/v1/alerts/'+office['id'], **auth)
        ack = request('POST', '/api/v1/alerts/'+office['id']+'/acknowledge', {'note': 'Owned zone trial'}, etag=current['etag'], **auth); assert ack['status'] == 200, ack
        assert state('research-web', 'server', 'normal')['acknowledgedAt'] is None
        binding = resource('AlertRule', rule['metadata']['name']); changed = copy.deepcopy(binding['body']['manifest']); changed['spec']['scope'] = 'monitor'; changed['spec']['when'] = 'monitor.state == "down"'
        blocked = request('PUT', '/api/v1/resources/AlertRule/'+rule['metadata']['name'], changed, etag=binding['etag'], **auth); assert blocked['status'] == 409, blocked
        deleted = request('DELETE', '/api/v1/resources/AlertRule/'+rule['metadata']['name'], etag=binding['etag'], **auth); assert deleted['status'] == 409, deleted
        cases['office-firing'] = {'alert': office, 'evidence': evidence, 'acknowledgement': ack['body'], 'bindingUpdate': blocked['status'], 'deleteRule': deleted['status']}; save()
        other = resource('Monitor', 'zone-other')['body']['manifest']; other['metadata']['labels']['environment'] = 'research'; put('Monitor', 'zone-other', other)
        other_office = lab.wait(lambda: state('zone-other', 'office', 'firing'), 'new selector member firing', seconds=60)
        current = resource('Monitor', 'zone-other'); blocked = request('DELETE', '/api/v1/resources/Monitor/zone-other', etag=current['etag'], **auth); assert blocked['status'] == 409, blocked
        other = current['body']['manifest']; other['metadata']['labels']['environment'] = 'other'; put('Monitor', 'zone-other', other)
        def deselected_still_firing():
            a = request('GET', '/api/v1/monitors/zone-other/assessment', **auth)['body']; current = resource('Monitor', 'zone-other')
            return state('zone-other', 'office', 'firing') if a.get('revision') == current['body']['status']['desiredRevision'] and any(z['zone'] == 'office' and z['state'] == 'down' for z in a.get('zoneStates', [])) else None
        held = lab.wait(deselected_still_firing, 'deselection is not recovery', seconds=60)
        assert held['id'] == other_office['id']
        cases['membership'] = {'newMember': other_office, 'deselectedStillFiring': held, 'deleteActiveMonitor': blocked['status']}; save()
        lab.inject('healthy')
        resolved = lab.wait(lambda: state('research-web', 'office', 'resolved'), 'primary recovered', seconds=60)
        other_resolved = lab.wait(lambda: state('zone-other', 'office', 'resolved'), 'deselected instance recovered', seconds=60)
        assert resolved['acknowledgedAt'] and state('research-web', 'server', 'normal')
        cases['resolved'] = {'primary': resolved, 'deselected': other_resolved}; save()
        lab.inject('office-path')
        repeated = lab.wait(lambda: state('research-web', 'office', 'firing'), 'new primary cycle', seconds=60)
        assert repeated['acknowledgedAt'] is None and repeated['id'] == office['id']
        assert state('zone-other', 'office', 'resolved'), 'nonmatching Monitor fired again'
        cases['new-cycle'] = repeated; save()
    finally:
        lab.inject('healthy')
    lab.wait(healthy, 'regular monitoring restored', seconds=60)
    lab.wait(lambda: state('research-web', 'office', 'resolved'), 'final primary resolution')
    def delivered():
        raw = lab.sql("SELECT COALESCE(json_agg(json_build_object('id',event_id,'state',state,'payload',payload)),'[]'::json) FROM nmo_webhook_outbox WHERE payload->>'alert_rule'='selected-zone-outage'")
        rows = json.loads(raw)
        return rows if len(rows) == 6 and all(r['state'] == 'delivered' for r in rows) else None
    deliveries = lab.wait(delivered, 'six durable zone deliveries', seconds=60)
    assert all(r['payload']['zone'] == 'office' for r in deliveries), deliveries
    current = resource('AlertRule', rule['metadata']['name']); deleted = request('DELETE', '/api/v1/resources/AlertRule/'+rule['metadata']['name'], etag=current['etag'], **auth); assert deleted['status'] == 204, deleted
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'cases': cases, 'apply': operation['body'], 'emptySelectorApply': empty_operation['body'], 'deliveries': deliveries, 'unknownZoneFieldRejected': 422, 'resolvedRuleDeleted': deleted['status'], 'regularHealthy': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('action', choices=['prepare', 'verify', 'down']); p.add_argument('--project', required=True); p.add_argument('--prefix', required=True); p.add_argument('--output')
    a = p.parse_args(); lab = PlacementLab(a.project, a.prefix)
    if a.action == 'prepare': lab.prepare_inventory()
    elif a.action == 'down': lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not a.output: p.error('--output required')
        output = Path(a.output)
        if output.exists(): raise FileExistsError('Use a new output path')
        lab.up(); result = verify(lab, output); output.write_text(json.dumps(result, indent=2)+'\n'); output.chmod(0o600); print('PASS: zonal alerts, selectors, scoped evidence, ACK and webhook lifecycle')
