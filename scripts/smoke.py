"""Exercise the built CLI against a real local HTTP server; no dependencies."""
import json
from pathlib import Path
import subprocess
import threading
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROOT = Path(__file__).resolve().parents[1]
BIN = ROOT / '.local/netmonorc'
parser = argparse.ArgumentParser()
parser.add_argument('--blackbox-binary')
args = parser.parse_args()
CONFIG = ROOT / 'examples/http.yaml'
if args.blackbox_binary:
    CONFIG = ROOT / '.local/blackbox-smoke.yaml'
    CONFIG.write_text((ROOT / 'examples/http.yaml').read_text().replace('runner: native-http', 'runner: blackbox'))

class Handler(BaseHTTPRequestHandler):
    status = 200

    def do_GET(self):
        if self.path == "/redirect":
            self.send_response(302)
            self.send_header("Location", "/")
            self.end_headers()
            return
        if self.path == "/large":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"x" * 20000)
            return
        self.send_response(self.status)
        self.end_headers()
        self.wfile.write(b'health')

    def log_message(self, *args):
        pass

def cli(*args, input=None):
    return subprocess.check_output([str(BIN), *args], cwd=ROOT, input=input)

def probe(agent, zone):
    extra = ['-blackbox-binary', args.blackbox_binary] if args.blackbox_binary else []
    return cli('probe', '-config', str(CONFIG), '-agent', agent, '-zone', zone,
               '-allow-cidrs', '127.0.0.1/32', *extra)

def assess(data):
    return json.loads(cli('assess', '-config', str(CONFIG), '-slots', 'branch=office,dc=server', input=data))

server = ThreadingHTTPServer(('127.0.0.1', 18080), Handler)
thread = threading.Thread(target=server.serve_forever, daemon=True)
thread.start()
try:
    office, dc = probe('branch', 'office'), probe('dc', 'server')
    assert assess(office + dc)['state'] == 'healthy'
    assert assess(dc)['state'] == 'unknown'
    Handler.status = 503
    failed = probe('branch', 'office')
    result = assess(dc + failed)
    assert result['state'] == 'degraded'
    assert result['zones'][0]['state'] == 'down'
    for path, expected in [('redirect', 'failure'), ('large', 'unknown')]:
        variant = ROOT / '.local/bounds-smoke.yaml'
        variant.write_text(CONFIG.read_text().replace('18080/', '18080/' + path))
        extra = ['-blackbox-binary', args.blackbox_binary] if args.blackbox_binary else []
        result = json.loads(cli('probe', '-config', str(variant), '-agent', 'branch',
                                '-zone', 'office', '-allow-cidrs', '127.0.0.1/32', *extra))
        assert result['outcome'] == expected, result
    Handler.status = 200
    variant = ROOT / '.local/body-regex-smoke.yaml'
    variant.write_text(CONFIG.read_text().replace('expectedStatus: [200]', "expectedStatus: [200]\n        bodyMatchesRegex: '^unavailable$'"))
    extra = ['-blackbox-binary', args.blackbox_binary] if args.blackbox_binary else []
    result = json.loads(cli('probe', '-config', str(variant), '-agent', 'branch', '-zone', 'office',
                            '-allow-cidrs', '127.0.0.1/32', *extra))
    assert result['execution'] == 'completed' and result['outcome'] == 'failure' and result['statusCode'] == 200, result
    print('PASS: HTTP 200 with mismatching body regex -> negative probe', flush=True)
    (ROOT / '.local/observations.ndjson').write_bytes(dc + failed)
    print('PASS: HTTP 200 -> healthy; missing office -> unknown; '
          'office HTTP 503 + server HTTP 200 -> degraded')
finally:
    server.shutdown()
    server.server_close()
    thread.join()
