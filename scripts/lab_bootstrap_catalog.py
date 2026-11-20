"""Owned multi-document YAML bootstrap: atomic refs, two Monitors and restart semantics."""
import argparse
from datetime import datetime, timezone
import io
import json
from pathlib import Path
import secrets
import subprocess
import tarfile
from lab import Lab, ROOT, manifests
from lab_replicas import CLIENT


class CatalogLab(Lab):
    def compose(self, *args, input=None, env=None):
        override = self.work/'catalog.compose.json'
        if not override.exists(): return super().compose(*args, input=input, env=env)
        return subprocess.run(['docker', 'compose', '-p', self.project, '-f', str(ROOT/'compose.yaml'), '-f', str(override), *args], cwd=ROOT, env={**self.env, **(env or {})}, input=input, stdin=subprocess.DEVNULL if input is None else None, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True).stdout

    def write_catalog(self, data):
        archive = io.BytesIO()
        with tarfile.open(fileobj=archive, mode='w') as tar:
            payload = data.encode(); item = tarfile.TarInfo('controller/monitor.yaml'); item.size = len(payload); item.mode = 0o600; item.uid = item.gid = 10001
            tar.addfile(item, io.BytesIO(payload))
        self.compose('run', '--rm', '--no-deps', '-T', 'init', input=archive.getvalue())

    def prepare_catalog(self):
        if self.work.exists(): raise RuntimeError('Use a new owned project')
        self.prepare()
        command = json.loads(self.compose('config', '--format', 'json'))['services']['controller']['command']
        command = [x for x in command if not x.startswith(('-diagnostic-profile=', '-alert-rule=', '-webhook-config='))]
        command.append('-monitor=research-web')
        (self.work/'catalog.compose.json').write_text(json.dumps({'services': {'controller': {'command': command}}}))
        def m(kind, name, spec): return json.dumps({'apiVersion': 'netmonorc.io/v1alpha1', 'kind': kind, 'metadata': {'name': name}, 'spec': spec})
        spec = {'targetRef': 'shared-target', 'placementRef': 'primary-office', 'useProfiles': [{'ref': 'http-checks', 'as': 'web', 'parameters': {'url': 'http://198.18.0.80:8080/'}}], 'assessmentRef': 'office-health', 'diagnostics': {'enabled': False, 'profileRef': 'web-lab'}}
        docs = [m('Monitor', 'research-web', spec), m('Monitor', 'research-alt', spec),
                m('CheckProfile', 'http-checks', {'parameters': [{'name': 'url', 'type': 'url', 'required': True}], 'checks': [{'id': 'http', 'type': 'http', 'runner': 'native-http', 'interval': '10s', 'timeout': '2s', 'config': {'url': '${params.url}', 'expectedStatus': [200]}}]}),
                m('Target', 'shared-target', {'type': 'host', 'address': '198.18.0.80'}),
                m('PlacementPolicy', 'primary-office', {'strategy': 'fixed', 'agentRefs': ['branch'], 'replicas': 1, 'zones': ['office']}),
                m('AssessmentPolicy', 'office-health', {'requiredZones': ['office'], 'freshness': {'maxAge': '30s', 'maxSkew': '15s'}, 'checks': [{'id': 'web__http', 'weight': 1, 'required': True}]}),
                m('Target', 'unused-target', {'type': 'logical'}), manifests()['adaptive-confirmed.yaml'], manifests()['alert.yaml'],
                m('WebhookEndpoint', 'default-webhook', {'url': 'https://'+self.prefix+'.40.6:9444/events', 'auth': {'type': 'bearer', 'secretRef': 'alert-webhook-token'}, 'timeout': '2s', 'retry': {'maxAttempts': 5, 'initialBackoff': '1s', 'maxBackoff': '4s'}})]
        (self.work/'catalog-documents.json').write_text(json.dumps(docs))
        # A missing ref in the second Monitor must leave zero persisted resources.
        bad = docs.copy(); broken = json.loads(bad[1]); broken['spec']['targetRef'] = 'missing-target'; bad[1] = json.dumps(broken)
        self.write_catalog('\n---\n'.join(bad))
        print('Prepared owned catalog with deliberate unresolved ref')


def verify(lab):
    lab.require(); docs = json.loads((lab.work/'catalog-documents.json').read_text())
    lab.compose('up', '-d', '--wait', '--wait-timeout', '90')
    def rejected():
        logs = lab.compose('logs', '--no-color', 'controller').decode()
        return 'bootstrap catalog:' in logs and 'missing-target' in logs
    lab.wait(rejected, 'invalid startup graph rejected before seed', seconds=30)
    counts = {table: int(lab.sql('SELECT count(*) FROM '+table)) for table in ['nmo_resources', 'nmo_resource_revisions', 'nmo_resource_bootstrap']}
    assert all(n == 0 for n in counts.values()), counts
    lab.write_catalog('\n---\n'.join(docs)); lab.compose('restart', 'controller')
    lab.wait(lambda: int(lab.sql('SELECT count(*) FROM nmo_resources')) == len(docs), 'whole valid catalog seeded', seconds=30)
    if lab.sql('SELECT count(*) FROM nmo_users') != '0': raise RuntimeError('Use a fresh owned project')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'catalog-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'catalog-admin', 'password': password}); password = ''; assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    def resource(kind, name):
        response = request('GET', '/api/v1/resources/'+kind+'/'+name, **auth); assert response['status'] == 200
        return response
    def healthy(name, since=None):
        def ready():
            r = resource('Monitor', name); response = request('GET', '/api/v1/monitors/'+name+'/assessment', **auth)
            if response['status'] != 200: return None
            a = response['body']
            return a if a['state'] == 'healthy' and a['revision'] == r['body']['status']['desiredRevision'] and (since is None or datetime.fromisoformat(a['evaluatedAt'].replace('Z', '+00:00')) > since) else None
        return lab.wait(ready, 'fresh exact revision '+name, seconds=60)
    initial = {name: healthy(name) for name in ['research-web', 'research-alt']}
    versions = {name: resource('Monitor', name)['etag'] for name in initial}
    assert int(lab.sql("SELECT count(*) FROM nmo_observations WHERE check_id='web__http' AND agent_id='branch' AND payload->>'outcome'='success'")) >= 2
    assert lab.sql("SELECT count(*) FROM nmo_observations WHERE agent_id='dc'") == '0', 'idle spare executed primary checks'
    target = resource('Target', 'shared-target'); changed = target['body']['manifest']; changed['spec']['criticality'] = 4
    update = request('PUT', '/api/v1/resources/Target/shared-target', changed, etag=target['etag'], **auth); assert update['status'] == 200
    edited_target_version = update['etag']
    updated = {name: healthy(name) for name in initial}
    assert all(updated[name]['revision'] != initial[name]['revision'] for name in initial)
    unused = resource('Target', 'unused-target')
    assert request('DELETE', '/api/v1/resources/Target/unused-target', etag=unused['etag'], **auth)['status'] == 204
    # All source docs remain present, in reverse order; DB edits and deletion win.
    lab.write_catalog('\n---\n'.join(reversed(docs)))
    since = datetime.now(timezone.utc); lab.compose('restart', 'controller')
    restored = {name: healthy(name, since) for name in initial}
    current_target = resource('Target', 'shared-target')
    assert current_target['etag'] == edited_target_version and current_target['body']['manifest']['spec']['criticality'] == 4
    assert request('GET', '/api/v1/resources/Target/unused-target', **auth)['status'] == 404
    assert all(restored[name]['revision'] == updated[name]['revision'] and resource('Monitor', name)['etag'] == versions[name] for name in initial)
    history = {table: int(lab.sql('SELECT count(*) FROM '+table)) for table in ['nmo_resources', 'nmo_resource_revisions', 'nmo_resource_bootstrap']}
    assert history['nmo_resources'] == len(docs)-1 and history['nmo_resource_bootstrap'] == len(docs)
    assert history['nmo_resource_revisions'] == len(docs)+1, 'restart rewrote source history'
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'documentCount': len(docs), 'zeroWritesOnBrokenReference': counts, 'firstHealthy': initial, 'afterTargetEdit': updated, 'freshAfterReversedCatalogRestart': restored, 'persistedTargetEditPreserved': True, 'deletedResourceNotResurrected': True, 'monitorVersionsUnchanged': True, 'history': history, 'realProbesPostgreSQLMTLSHTTPS': True, 'legacyProfileAlertEndpointFlagsRemoved': True, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('action', choices=['prepare', 'verify', 'down']); p.add_argument('--project', required=True); p.add_argument('--prefix', required=True); p.add_argument('--output')
    a = p.parse_args(); lab = CatalogLab(a.project, a.prefix)
    if a.action == 'prepare': lab.prepare_catalog()
    elif a.action == 'down': lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not a.output: p.error('--output required')
        path = Path(a.output)
        if path.exists(): raise FileExistsError('Use a new output path')
        result = verify(lab); path.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
        with path.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
        path.chmod(0o600); print('PASS: atomic YAML graph, both Monitors, persisted edits and tombstones across reordered restart')
