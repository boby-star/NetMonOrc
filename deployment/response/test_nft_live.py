"""Run ONLY in a dedicated container network namespace with CAP_NET_ADMIN."""
import http.client
import json
import socket
import subprocess
import sys
import tempfile
import threading
import time
from pathlib import Path
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def check(source, port):
    try:
        with socket.create_connection(('198.18.0.1', port), timeout=0.3, source_address=(source, 0)):
            return True
    except OSError:
        return False

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'fixture')
    def log_message(self, *args):
        pass

for address in ['198.18.0.1', '198.18.0.2', '198.18.0.3']:
    subprocess.run(['ip', 'addr', 'add', address + '/32', 'dev', 'lo'], check=True)
subprocess.run(['ip', 'link', 'set', 'lo', 'up'], check=True)
servers = [ThreadingHTTPServer(('198.18.0.1', port), Handler) for port in [18088, 18090]]
for server in servers:
    server.daemon_threads = True
    threading.Thread(target=server.serve_forever, daemon=True).start()
assert check('198.18.0.2', 18088) and check('198.18.0.3', 18088)
with tempfile.TemporaryDirectory() as tmp:
    sock = str(Path(tmp) / 'helper.sock')
    process = subprocess.Popen([sys.executable, '/test/helper.py', '--ports', '18088', '--sources', '198.18.0.2/32', '--agent-uid', '0', '--socket-gid', '0', '--socket', sock, '--state-file', str(Path(tmp) / 'journal.json')])
    try:
        for _ in range(100):
            if Path(sock).exists():
                break
            if process.poll() is not None:
                raise RuntimeError('helper startup failed')
            time.sleep(0.05)
        until = int(time.time() * 1000) + 5000
        client = http.client.HTTPConnection('localhost', timeout=2)
        client.sock = socket.socket(socket.AF_UNIX)
        client.sock.connect(sock)
        client.request('POST', '/block', json.dumps({'id': 'a' * 32, 'port': 18088, 'sourceCIDR': '198.18.0.2/32', 'untilUnixMilli': until}), {'Content-Type': 'application/json'})
        result = json.loads(client.getresponse().read())
        client.close()
        assert result['status'] == 'applied', result
        assert not check('198.18.0.2', 18088), 'selected source was not blocked'
        assert check('198.18.0.3', 18088), 'other source affected'
        assert check('198.18.0.2', 18090), 'other port affected'
        process.kill()
        process.wait(timeout=2)
        assert not check('198.18.0.2', 18088), 'block lost on helper crash'
        while int(time.time() * 1000) < until + 700:
            time.sleep(0.1)
        assert check('198.18.0.2', 18088), 'kernel timeout did not restore access'
        print(json.dumps({'status': 'passed', 'blockedOnlySelectedSourceAndPort': True, 'survivedHelperCrash': True, 'accessRestoredByKernelTTL': True}))
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
for server in servers:
    server.shutdown()
    server.server_close()
