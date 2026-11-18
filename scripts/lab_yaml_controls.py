"""Owned YAML controls acceptance: revision canonicalization, regular checks and disable/re-enable."""
import argparse
import copy
import json
from pathlib import Path
import secrets
import time
from lab import Lab
from lab_replicas import CLIENT


def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users') != '0':
        raise RuntimeError('Use a fresh owned project; existing users are preserved')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'yaml-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'yaml-admin', 'password': password})
    password = ''
    assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    monitor_path = '/api/v1/resources/Monitor/research-web'
    profile_path = '/api/v1/resources/DiagnosticProfile/web-lab'
    original_monitor = request('GET', monitor_path, **auth)
    original_profile = request('GET', profile_path, **auth)
    assert original_monitor['status'] == original_profile['status'] == 200
    initial_revision = lab.assessment()['revision']
    def replace(path, manifest):
        current = request('GET', path, **auth)
        response = request('PUT', path, manifest, etag=current['etag'], **auth)
        assert response['status'] == 200, response['status']
        return response['body']['status']['desiredRevision']
    def assessed(revision, state):
        def matches():
            value = lab.assessment()
            return value if value.get('revision') == revision and value.get('state') == state else None
        return lab.wait(matches, state+' under exact requested revision', seconds=65)
    cases = {}
    try:
        reordered = copy.deepcopy(original_profile['body']['manifest'])
        reordered['spec']['actions'].reverse()
        revision = replace(profile_path, reordered)
        assert revision == original_profile['body']['status']['desiredRevision'], 'action permutation changed profile hash'
        assessed(initial_revision, 'healthy')
        cases['canonicalProfile'] = {'sameProfileRevision': True, 'sameMonitorRevision': True}
        modified = copy.deepcopy(original_monitor['body']['manifest'])
        for check in modified['spec']['checks']:
            if check['id'] == 'http': check.update({'priority': 90, 'jitter': '1s'})
            if check['id'] == 'tcp': check['priority'] = 10
            if check['id'] == 'dns': check['enabled'] = False
        modified['spec']['assessment']['checks'] = [c for c in modified['spec']['assessment']['checks'] if c['id'] != 'dns']
        revision = replace(monitor_path, modified)
        assessment = assessed(revision, 'healthy')
        assert len(assessment['zones']) == 2 and all(z['coverage'] == 'complete' for z in assessment['zones'])
        assert lab.sql("SELECT count(*) FROM nmo_observations WHERE revision='"+revision+"' AND check_id='dns'") == '0'
        assert lab.sql("SELECT count(DISTINCT check_id) FROM nmo_observations WHERE revision='"+revision+"' AND diagnostic_task_id=''") == '2'
        bundle = json.dumps(modified)+'\n---\n'+json.dumps(original_profile['body']['manifest'])
        validated = json.loads(lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc', 'validate', '-f', '-', '--slots', 'branch=office,dc=server', input=bundle.encode()))
        assert validated['valid'] and validated['mode'] == 'offline' and validated['planningRequired'] and len(validated['resources']) == 2
        cases['regularControls'] = {'healthy': True, 'completeZones': 2, 'enabledCheckTypesObserved': 2, 'disabledDNSObservations': 0, 'productionCLIStdinBatchValidated': True}
        disabled = copy.deepcopy(modified); disabled['spec']['enabled'] = False
        revision = replace(monitor_path, disabled)
        assessment = assessed(revision, 'unknown')
        assert all(z['reason'] == 'monitor_disabled' for z in assessment['zones'])
        assert lab.sql("SELECT count(*) FROM nmo_observations WHERE revision='"+revision+"' AND diagnostic_task_id=''") == '0'
        lab.wait(lambda: request('GET', monitor_path, **auth)['body']['status']['state'] == 'disabled', 'disabled resource status')
        rejected = request('POST', '/api/v1/monitors/research-web/diagnostics', {'profileRef': 'web-lab', 'requestId': secrets.token_hex(16)}, **auth)
        assert rejected['status'] == 409
        cases['disabledMonitor'] = {'state': 'unknown', 'reason': 'monitor_disabled', 'regularObservations': 0, 'manualDiagnosticRejected': True, 'resourceState': 'disabled'}
    finally:
        restored = replace(monitor_path, original_monitor['body']['manifest'])
        assessed(restored, 'healthy')
        replace(profile_path, original_profile['body']['manifest'])
    assessed(restored, 'healthy')
    lab.wait(lambda: request('GET', '/api/v1/auth/me', **auth)['status'] == 200, 'restored HTTP runtime')
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'realPostgreSQLAndAgentProbes': True, 'trustedHTTPS': True, 'cases': cases, 'restoredHealthy': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


def verify_upgrade(lab):
    """Exercise a stale derived profile hash only on this script's owned fixture."""
    lab.require()
    if lab.sql("SELECT count(*) FROM nmo_users") != '1' or lab.sql("SELECT count(*) FROM nmo_users WHERE username='yaml-admin'") != '1':
        raise RuntimeError('Requires the dedicated yaml-admin fixture from verify()')
    snapshot = "SELECT json_build_object('revision',effective_revision,'generation',generation,'version',version,'manifestDigest',md5(manifest::text))::text FROM nmo_resources WHERE kind='DiagnosticProfile' AND name='web-lab'"
    before = json.loads(lab.sql(snapshot))
    assert len(before['revision']) == 64
    lab.compose('stop', 'controller')
    try:
        lab.sql("UPDATE nmo_resources SET effective_revision=repeat('0',64) WHERE kind='DiagnosticProfile' AND name='web-lab'")
        started = time.time()
        lab.compose('up', '-d', '--no-deps', 'controller')
        lab.wait(lambda: json.loads(lab.sql(snapshot))['revision'] == before['revision'], 'canonical hash repaired after compiler upgrade', seconds=60)
        lab.wait(lambda: lab.assessment().get('state') == 'healthy' and int(lab.sql("SELECT count(*) FROM nmo_observations WHERE diagnostic_task_id='' AND received_at>to_timestamp("+str(started)+")")) >= 6, 'fresh probes after upgraded Controller restart', seconds=60)
        after = json.loads(lab.sql(snapshot))
        assert before == after, 'source version/generation/manifest changed during hash repair'
    except BaseException:
        lab.sql("UPDATE nmo_resources SET effective_revision='"+before['revision']+"' WHERE kind='DiagnosticProfile' AND name='web-lab'")
        lab.compose('up', '-d', '--no-deps', 'controller')
        raise
    return {'status': 'passed', 'staleDerivedHashRepaired': True, 'sourceVersionGenerationAndManifestUnchanged': True, 'freshHealthyAfterRestart': True, 'credentialsOmitted': True}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--upgrade-only', action='store_true', help='repair-check a stale derived hash on the dedicated yaml-admin fixture'); parser.add_argument('--project', required=True); parser.add_argument('--prefix', required=True); parser.add_argument('--output', required=True)
    args = parser.parse_args(); path = Path(args.output)
    if path.exists(): raise FileExistsError('Use a new output path')
    result = (verify_upgrade if args.upgrade_only else verify)(Lab(args.project, args.prefix))
    path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
    with path.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
    path.chmod(0o600)
    print('PASS: canonical profile revision, priority/jitter checks, disable/restore and offline CLI batch validation; secrets omitted')
