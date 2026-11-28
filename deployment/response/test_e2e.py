"""Owned container only: real Controller/PostgreSQL/mTLS Agent/nft helper."""
import http.server
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import ssl
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
from dev_pki import generate


def wait(label, function, seconds=35):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = function()
        if value:
            return value
        time.sleep(0.15)
    raise RuntimeError('timeout: ' + label)


def connect_source(port=18088):
    try:
        with socket.create_connection(('198.18.0.1', port), timeout=0.2, source_address=('198.18.0.2', 0)):
            return True
    except OSError:
        return False

exposed = False
class Target(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = b'exposure=public-debug\n' if exposed else b'exposure=restricted\n'
        self.send_response(200)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *args):
        pass

for address in ['198.18.0.1', '198.18.0.2']:
    subprocess.run(['ip', 'addr', 'add', address + '/32', 'dev', 'lo'], check=True)
subprocess.run(['ip', 'link', 'set', 'lo', 'up'], check=True)
service = http.server.ThreadingHTTPServer(('198.18.0.1', 18088), Target)
service.daemon_threads = True
threading.Thread(target=service.serve_forever, daemon=True).start()
processes, logs = [], []
with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    root.chmod(0o755)
    generate(root/'pki', agents=('branch',))
    for name in ['agent-pki', 'agent-data']:
        path = root/name
        path.mkdir(mode=0o700)
        os.chown(path, 10001, 10001)
    for name in ['ca.crt', 'branch.crt', 'branch.key']:
        path = root/'agent-pki'/name
        shutil.copy(root/'pki'/name, path)
        os.chown(path, 10001, 10001)
    dsn = root/'database.dsn'
    dsn.write_text(os.environ['NMO_TEST_DSN'])
    dsn.chmod(0o600)
    token = secrets.token_urlsafe(32)
    (root/'token').write_text(token)
    (root/'token').chmod(0o600)
    manifest = Path('/test/example.yaml').read_text().replace('188.137.224.35', '198.18.0.1').replace('188.137.254.31/32', '198.18.0.2/32')
    manifest = manifest.replace('agentRefs: [branch, dc], replicas: 2, distinctBy: zone', 'agentRefs: [branch], replicas: 1')
    manifest = manifest.replace('requiredZones: [office, server]', 'requiredZones: [office]').replace('duration: 60s', 'duration: 5s')
    (root/'monitor.yaml').write_text(manifest)
    def start(name, args, agent=False):
        output = open(root/(name+'.log'), 'wb')
        logs.append(output)
        def drop():
            os.setgroups([])
            os.setgid(10001)
            os.setuid(10001)
        proc = subprocess.Popen(args, stdout=output, stderr=subprocess.STDOUT,
                                preexec_fn=drop if agent else None)
        processes.append(proc)
        return proc
    def read(route):
        try:
            request = urllib.request.Request('https://127.0.0.1:18443/api/v1/monitors/security-exposure-demo/' + route,
                                             headers={'Authorization': 'Bearer ' + token})
            with urllib.request.urlopen(request, context=ssl.create_default_context(cafile=str(root/'pki/ca.crt')), timeout=2) as response:
                return json.load(response)
        except (OSError, urllib.error.URLError):
            return {}
    try:
        helper = start('helper', ['python3', '/test/helper.py', '--ports', '18088', '--sources', '198.18.0.2/32', '--state-file', str(root/'helper-state/journal.json')])
        wait('helper socket', lambda: Path('/run/netmonorc-response/helper.sock').exists())
        controller = start('controller', ['/test/bins/controller', '-config='+str(root/'monitor.yaml'), '-database-file='+str(dsn), '-slots=branch=office', '-listen=127.0.0.1:19443', '-ca='+str(root/'pki/ca.crt'), '-cert='+str(root/'pki/controller.crt'), '-key='+str(root/'pki/controller.key'), '-assessment-file='+str(root/'assessment.json'), '-http-listen=127.0.0.1:18443', '-http-token-file='+str(root/'token')])
        agent = start('agent', ['/test/bins/agent', '-controller=127.0.0.1:19443', '-server-name=localhost', '-ca='+str(root/'agent-pki/ca.crt'), '-cert='+str(root/'agent-pki/branch.crt'), '-key='+str(root/'agent-pki/branch.key'), '-data-dir='+str(root/'agent-data'), '-allow-cidrs=198.18.0.1/32', '-response-socket=/run/netmonorc-response/helper.sock'], agent=True)
        wait('initial healthy', lambda: read('assessment').get('state') == 'healthy')
        assert connect_source()
        assert not read('responses').get('items')
        exposed = True
        events = wait('applied response', lambda: (value if (value := read('responses').get('items')) and value[0]['status'] == 'applied' else None))
        assert not connect_source(), 'Controller response did not block source'
        # An unprivileged real Agent must remain connected while a different source is blocked.
        assert agent.poll() is None and controller.poll() is None and helper.poll() is None
        assert 'CapEff:\t0000000000000000' in Path(f'/proc/{agent.pid}/status').read_text()
        time.sleep(6)
        assert connect_source(), 'kernel TTL did not restore source'
        events = read('responses')['items']
        assert len(events) == 1 and events[0]['status'] == 'expired', events
        time.sleep(2)
        assert len(read('responses')['items']) == 1, 'same incident repeatedly blocked'
        exposed = False
        wait('healthy recovery', lambda: read('assessment').get('state') == 'healthy')
        time.sleep(1.5)
        exposed = True
        wait('new incident rearmed', lambda: len(read('responses').get('items', [])) == 2)
        print(json.dumps({'status': 'passed', 'agentUID': 10001, 'transport': 'real-mTLS', 'storage': 'PostgreSQL', 'automaticResponseApplied': True, 'kernelTTLRestoredAccess': True, 'oneCommandPerIncident': True, 'healthyRecoveryRearmed': True, 'historyEndpoint': True}))
    except Exception:
        for path in root.glob('*.log'):
            print(path.name + ':\n' + path.read_text(errors='replace')[-7000:])
        raise
    finally:
        for proc in reversed(processes):
            proc.terminate()
        for proc in reversed(processes):
            try:
                proc.wait(timeout=5)
            except subprocess.TimeoutExpired:
                proc.kill(); proc.wait()
        for log in logs:
            log.close()
        service.shutdown(); service.server_close()
