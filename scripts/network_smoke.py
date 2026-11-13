"""Exercise all four protocols against real Blackbox Exporter 0.27.0."""
import argparse
import json
from pathlib import Path
import socket
import subprocess
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from network_fixtures import DNSFixture

ROOT = Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('--blackbox-binary', required=True)
args = parser.parse_args()
(ROOT/'.local').mkdir(exist_ok=True)
dns = DNSFixture()
class Handler(BaseHTTPRequestHandler):
    status = 200
    def do_GET(self):
        self.send_response(self.status)
        self.end_headers()
        self.wfile.write(b'health')
    def log_message(self, *args):
        pass
http = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
thread = threading.Thread(target=http.serve_forever, daemon=True)
thread.start()
manifest = ROOT/'.local/network-smoke.yaml'
base = (ROOT/'examples/network.yaml').read_text().replace('18080', str(http.server_port)).replace('1053', str(dns.port))
manifest.write_text(base)

def probe(agent='branch', zone='office'):
    output = subprocess.check_output([str(ROOT/'.local/netmonorc'), 'probe', '-config', str(manifest),
                                     '-agent', agent, '-zone', zone, '-allow-cidrs', '127.0.0.1/32',
                                     '-blackbox-binary', args.blackbox_binary], cwd=ROOT)
    return output, {o['check']: o for o in map(json.loads, output.splitlines())}

try:
    office, rows = probe()
    for kind in ['http', 'tcp', 'dns']:
        assert rows[kind]['outcome'] == 'success', rows[kind]
    if rows['icmp']['outcome'] != 'success':
        assert rows['icmp']['execution'] == 'execution_failed' and rows['icmp']['reason'] == 'icmp_execution_unavailable', rows['icmp']
        print('SKIP: positive ICMP on host lacks socket permissions; Docker ICMP check is separate', flush=True)
    server, _ = probe('dc', 'server')
    result = json.loads(subprocess.check_output([str(ROOT/'.local/netmonorc'), 'assess', '-config', str(manifest),
                                               '-slots', 'branch=office,dc=server'], input=office+server, cwd=ROOT))
    assert result['state'] == 'healthy', result
    print('PASS: HTTP/TCP/DNS via real exporter; two labelled sources -> healthy', flush=True)
    print('PASS: DNS regex matched the second of two answer records', flush=True)
    dns.mode = 'empty'
    _, rows = probe()
    assert rows['dns']['outcome'] == 'failure', rows['dns']
    dns.mode = 'nxdomain'
    _, rows = probe()
    assert rows['dns']['outcome'] == 'failure', rows['dns']
    print('PASS: empty DNS answers and unexpected NXDOMAIN -> failure', flush=True)
    dns.mode = 'answers'
    manifest.write_text(base.replace('transport: udp', 'transport: tcp'))
    _, rows = probe()
    assert rows['dns']['outcome'] == 'success', rows['dns']
    print('PASS: DNS over TCP', flush=True)
    dns.mode = 'nxdomain'
    variant = base.replace("        expectedRecordRegex: '127\\.0\\.0\\.20$'", '        expectedRcodes: [NXDOMAIN]')
    manifest.write_text(variant)
    _, rows = probe()
    assert rows['dns']['outcome'] == 'success', rows['dns']
    print('PASS: explicitly expected NXDOMAIN -> success', flush=True)
    dns.mode = 'answers'
    # A bound but non-listening local TCP socket guarantees refusal, without
    # modifying host firewall rules or probing an unrelated external target.
    with socket.socket() as closed:
        closed.bind(('127.0.0.1', 0))
        manifest.write_text(base.replace('port: '+str(http.server_port), 'port: '+str(closed.getsockname()[1])))
        _, rows = probe()
        assert rows['tcp']['execution'] == 'completed' and rows['tcp']['outcome'] == 'failure', rows['tcp']
    print('PASS: closed TCP port -> negative probe, not execution failure', flush=True)
finally:
    dns.close()
    http.shutdown(); http.server_close(); thread.join()
