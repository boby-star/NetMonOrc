"""Owned HTTPS/Agent trial of static CEL check reference validation (DSL SC-18)."""
import argparse
import copy
import json
from pathlib import Path
import secrets
import subprocess

from lab import Lab, ROOT
from lab_replicas import CLIENT


def verify(lab):
    if lab.sql('SELECT count(*) FROM nmo_users') != '0':
        raise RuntimeError('Use a fresh owned project; existing users preserved')
    password = secrets.token_urlsafe(32)
    lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'cel-admin', '-password-stdin', input=password.encode())
    base = {'address': lab.prefix+'.40.4', 'ca': (lab.work/'pki/ca.crt').read_text()}
    def request(method, path, body=None, **extra):
        return json.loads(lab.compose('exec', '-T', 'office-net', 'python3', '-c', CLIENT, input=json.dumps({**base, 'method': method, 'path': path, 'body': body, **extra}).encode()))
    login = request('POST', '/api/v1/auth/login', {'username': 'cel-admin', 'password': password})
    password = ''
    assert login['status'] == 200
    auth = {'cookie': login['cookie'], 'csrf': login['body']['csrfToken']}
    path = '/api/v1/resources/Monitor/research-web'
    original = request('GET', path, **auth)
    assert original['status'] == 200
    bad = copy.deepcopy(original['body']['manifest'])
    bad['spec']['assessment']['rules'] = [{'id': 'http-down', 'when': 'checks["htttp"].outcome == "failure"', 'then': {'state': 'down', 'reason': 'HTTP_FAILED'}}]
    batch = {'manifests': [bad], 'requestId': secrets.token_hex(16), 'expectedVersions': {'Monitor/research-web': original['body']['resourceVersion']}}
    validation = request('POST', '/api/v1/config/validate', batch, **auth)
    assert validation['status'] == 200 and not validation['body']['valid'], validation
    assert 'unknown check' in json.dumps(validation['body']), validation
    plan = request('POST', '/api/v1/config/plan', batch, **auth)
    apply = request('POST', '/api/v1/config/apply', batch, **auth)
    single = request('PUT', path, bad, etag=original['etag'], **auth)
    assert plan['status'] == apply['status'] == single['status'] == 422, (plan, apply, single)
    unchanged = request('GET', path, **auth)
    assert unchanged['etag'] == original['etag'] and unchanged['body']['status']['desiredRevision'] == original['body']['status']['desiredRevision']
    # Production offline CLI reads the same manifest from stdin; no credentials.
    cli_args = ('exec', '-T', 'controller', '/usr/local/bin/netmonorc', 'validate', '--config', '-')
    cli_bad = copy.deepcopy(bad)
    cli_bad['spec']['diagnostics'] = {'enabled': False}
    try:
        lab.compose(*cli_args, input=json.dumps(cli_bad).encode())
        raise AssertionError('offline CLI admitted unknown check')
    except subprocess.CalledProcessError as error:
        assert b'unknown check' in error.stderr, error.stderr
    good = copy.deepcopy(bad)
    good['spec']['assessment']['rules'][0]['when'] = 'checks["http"].outcome == "failure"'
    cli_manifest = copy.deepcopy(good)
    cli_manifest['spec']['diagnostics'] = {'enabled': False}
    cli_good = json.loads(lab.compose(*cli_args, input=json.dumps(cli_manifest).encode()))
    assert cli_good['valid']
    example = (ROOT/'examples/cel-check-references.yaml').read_bytes()
    example_validation = json.loads(lab.compose(*cli_args, '--slots', 'branch=office', input=example))
    assert example_validation['valid']
    try:
        lab.compose(*cli_args, '--slots', 'branch=office', input=example.replace(b'web__http', b'web__htttp'))
        raise AssertionError('expanded YAML profile typo admitted')
    except subprocess.CalledProcessError as error:
        assert b'unknown check' in error.stderr, error.stderr

    updated = request('PUT', path, good, etag=original['etag'], **auth)
    assert updated['status'] == 200, updated
    revision = updated['body']['status']['desiredRevision']
    def healthy():
        response = request('GET', '/api/v1/monitors/research-web/assessment', **auth)
        body = response['body']
        return body if response['status'] == 200 and body['revision'] == revision and body['state'] == 'healthy' else None
    assessment = lab.wait(healthy, 'healthy real probes after corrected CEL', seconds=60)
    assert request('POST', '/api/v1/auth/logout', **auth)['status'] == 204
    return {'status': 'passed', 'validation': validation['body'], 'rejectionStatuses': {'plan': plan['status'], 'apply': apply['status'], 'put': single['status']}, 'invalidWritePreservedETagAndRevision': True, 'offlineCLIRejectedTypo': True, 'offlineCLIAcceptedCorrected': cli_good, 'yamlProfileCatalogValidated': example_validation, 'yamlExpandedCheckTypoRejected': True, 'correctedRevision': revision, 'assessment': assessment, 'browserInteractionTested': False, 'credentialsOmitted': True}


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('action', choices=['prepare', 'verify', 'down'])
    p.add_argument('--project', required=True)
    p.add_argument('--prefix', required=True)
    p.add_argument('--output')
    args = p.parse_args()
    lab = Lab(args.project, args.prefix)
    if args.action == 'prepare':
        if lab.work.exists(): raise RuntimeError('Use a new owned project')
        lab.prepare()
    elif args.action == 'down':
        lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not args.output: p.error('--output required')
        output = Path(args.output)
        if output.exists(): raise FileExistsError('Use a new output path')
        lab.up(); result = verify(lab)
        output.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        with output.open('x') as f: json.dump(result, f, indent=2); f.write('\n')
        output.chmod(0o600)
        print('PASS: invalid CEL refs rejected before writes; corrected config runs healthy')
