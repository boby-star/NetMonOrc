"""Real browser YAML package admission on an owned HTTPS stand."""
import argparse
import json
from pathlib import Path
import secrets
import subprocess
from lab import Lab, ROOT

YAML = '''apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata: {name: yaml-web}
spec:
  targetRef: yaml-target
  placement: {strategy: fixed, agentRefs: [branch], replicas: 1}
  useProfiles:
    - ref: yaml-http
      as: web
      parameters: {url: 'http://198.18.0.80:8080/', ratio: 1.0, status: 200}
  assessment:
    requiredZones: [office]
    freshness: {maxAge: 30s, maxSkew: 15s}
    checks: [{id: web__http, weight: 1, required: true}]
---
apiVersion: netmonorc.io/v1alpha1
kind: CheckProfile
metadata: {name: yaml-http}
spec:
  parameters:
    - {name: url, type: url, required: true}
    - {name: ratio, type: float, required: true}
    - {name: status, type: integer, default: 200}
  checks:
    - id: http
      type: http
      runner: native-http
      interval: 10s
      timeout: 2s
      config: {url: '${params.url}', expectedStatus: ['${params.status}']}
---
'''
TARGET = '''apiVersion: netmonorc.io/v1alpha1
kind: Target
metadata: {name: yaml-target}
spec:
  type: host
  address: 198.18.0.80
  criticality: 3
  attributes: {ratio: 1.0, counter: 9007199254740993}
'''


def verify(lab, output, screenshot, single_editor=False):
    lab.require()
    credentials = lab.work/'yaml-browser-credentials.json'
    if lab.sql('SELECT count(*) FROM nmo_users') != '0':
        if not credentials.exists() or lab.sql("SELECT count(*) FROM nmo_resources WHERE name IN ('yaml-web','yaml-http','yaml-target')") != '0':
            raise RuntimeError('Use a fresh owned project; existing users/resources preserved')
        password = json.loads(credentials.read_text())['password']
    else:
        password = secrets.token_urlsafe(32)
        lab.compose('exec', '-T', 'controller', '/usr/local/bin/netmonorc-admin', '-database-file', '/run/netmonorc/database.dsn', '-username', 'yaml-admin', '-password-stdin', input=password.encode())
        with credentials.open('x') as private: json.dump({'password': password},private)
        credentials.chmod(0o600)
    config = {'origin': 'https://localhost:'+str(lab.ui_port()), 'username': 'yaml-admin', 'password': password, 'caFile': str(lab.work/'pki/ca.crt'), 'certFile': str(lab.work/'pki/controller.crt'), 'yaml': YAML+TARGET, 'targetYaml': TARGET, 'badIntegerYaml': (YAML+TARGET).replace('status: 200}', 'status: 200.0}'), 'singleEditor': single_editor, 'output': str(output.resolve()), 'screenshot': str(screenshot.resolve())}
    subprocess.run(['node', str(ROOT/'scripts/live_yaml_browser.mjs')], input=json.dumps(config).encode(), check=True, timeout=300)
    password = ''; config['password'] = ''
    result = json.loads(output.read_text())
    key = result['preparedRequestId']; assert len(key) == 32 and all(c in '0123456789abcdef' for c in key)
    count = lab.sql("SELECT count(*) FROM nmo_operations WHERE request_key='"+key+"'")
    assert count == '1', 'same request created more than one operation'
    result['durableOperationCountForReplay'] = int(count)
    result['targetCounterExactInPostgreSQL'] = lab.sql("SELECT manifest->'spec'->'attributes'->>'counter' FROM nmo_resources WHERE kind='Target' AND name='yaml-target'")
    assert result['targetCounterExactInPostgreSQL'] == '9007199254740993'
    result['newMonitorSourceHistoryCount'] = int(lab.sql("SELECT count(*) FROM nmo_resource_revisions WHERE manifest->>'kind'='Monitor' AND manifest->'metadata'->>'name'='yaml-web'"))
    assert result['newMonitorSourceHistoryCount'] == 1
    if single_editor:
        result['singleTargetHistoryCount'] = int(lab.sql("SELECT count(*) FROM nmo_resource_revisions WHERE manifest->>'kind'='Target' AND manifest->'metadata'->>'name'='yaml-target'"))
        assert result['singleTargetHistoryCount'] == 3, 'stale single-resource save changed history'
    output.write_text(json.dumps(result, indent=2)+'\n'); output.chmod(0o600)
    print('PASS: real browser YAML package, durable single replay operation, exact integer and source history')


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__); p.add_argument('action', choices=['prepare', 'verify', 'down']); p.add_argument('--project', required=True); p.add_argument('--prefix', required=True); p.add_argument('--port', type=int, default=8447); p.add_argument('--output'); p.add_argument('--screenshot'); p.add_argument('--single-editor', action='store_true')
    a = p.parse_args(); lab = Lab(a.project, a.prefix)
    if a.action == 'prepare':
        if lab.work.exists(): raise RuntimeError('Use a new owned project')
        lab.prepare(); lab.configure_ui(a.port)
    elif a.action == 'down': lab.require(); lab.compose('down'); print('Stopped; .local, PKI and volumes preserved')
    else:
        if not a.output or not a.screenshot: p.error('--output and --screenshot required')
        output = Path(a.output); screenshot = Path(a.screenshot)
        if output.exists() or screenshot.exists(): raise FileExistsError('Use new output paths')
        output.parent.mkdir(parents=True, mode=0o700, exist_ok=True); screenshot.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
        lab.up(); verify(lab, output, screenshot, a.single_editor)
