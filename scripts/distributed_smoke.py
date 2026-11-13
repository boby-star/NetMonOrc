"""Real PostgreSQL + Controller + two mTLS Agent processes. Own resources only."""
import json
import argparse
from network_fixtures import DNSFixture
import os
from pathlib import Path
import secrets
import socket
import shutil
import ssl
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from dev_pki import generate

ROOT = Path(__file__).resolve().parents[1]
IMAGE = 'postgres:17.6-alpine@sha256:ef257d85f76e48da1c64832459b59fcaba1a4dac97bf5d7450c77753542eee94'
work = ROOT/'.local'/('distributed-' + secrets.token_hex(4))
work.mkdir(parents=True, mode=0o700)
container = 'netmonorc-smoke-' + secrets.token_hex(4)
parser = argparse.ArgumentParser()
parser.add_argument('--network', action='store_true')
parser.add_argument('--blackbox-binary')
args = parser.parse_args()
if args.network and not args.blackbox_binary:
    parser.error('--network requires --blackbox-binary')
dns = DNSFixture() if args.network else None
processes = []
files = []
owned_container = False

def command(*args, **kwargs):
    return subprocess.check_output(args, cwd=ROOT, stderr=subprocess.PIPE, **kwargs).decode().strip()

def wait_for(description, check, seconds=30):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        value = check()
        if value:
            print('PASS:', description, flush=True)
            return value
        time.sleep(.2)
    raise RuntimeError(f'timed out: {description}; logs in {work}')

def start(name, *args):
    out = open(work/(name+'.log'), 'ab')
    files.append(out)
    p = subprocess.Popen([str(ROOT/'.local'/name.split('-run')[0]), *args],
                         cwd=ROOT, stdout=out, stderr=subprocess.STDOUT)
    processes.append(p)
    return p

def stop(p):
    if p.poll() is None:
        p.terminate()
        try:
            p.wait(8)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait()

def sql(query):
    return command('docker', 'exec', container, 'psql', '-U', 'netmonorc', '-d', 'netmonorc', '-tAc', query)

def view():
    try:
        return json.loads((work/'assessment.json').read_text())
    except (FileNotFoundError, json.JSONDecodeError):
        return {}

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
try:
    generate(work/'pki')
    password = secrets.token_hex(24)
    env = work/'postgres.env'
    env.write_text(f'POSTGRES_USER=netmonorc\nPOSTGRES_DB=netmonorc\nPOSTGRES_PASSWORD={password}\n')
    env.chmod(0o600)
    command('docker', 'run', '-d', '--name', container, '--env-file', str(env),
            '-p', '127.0.0.1::5432', IMAGE)
    owned_container = True
    port = command('docker', 'port', container, '5432/tcp').rsplit(':', 1)[1]
    dsn = work/'postgres.dsn'
    dsn.write_text(f'postgresql://netmonorc:{password}@127.0.0.1:{port}/netmonorc?sslmode=disable')
    dsn.chmod(0o600)
    def db_ready():
        try:
            return command('docker', 'exec', container, 'pg_isready', '-h', '127.0.0.1', '-U', 'netmonorc')
        except subprocess.CalledProcessError:
            return False
    wait_for('PostgreSQL ready', db_ready)
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        rpc_port = sock.getsockname()[1]
    manifest = work/'monitor.yaml'
    example = 'network.yaml' if args.network else 'http.yaml'
    text = (ROOT/'examples'/example).read_text().replace('18080', str(http.server_port))
    if dns:
        text = text.replace('1053', str(dns.port))
    text = text.replace('interval: 30s', 'interval: 10s').replace('maxAge: 90s', 'maxAge: 20s')
    manifest.write_text(text)
    pki = work/'pki'
    controller_args = ['-config', str(manifest), '-listen', f'127.0.0.1:{rpc_port}',
                       '-database-file', str(dsn), '-slots', 'branch=office,dc=server',
                       '-ca', str(pki/'ca.crt'), '-cert', str(pki/'controller.crt'),
                       '-key', str(pki/'controller.key'), '-assessment-file', str(work/'assessment.json')]
    controller = start('netmonorc-controller', *controller_args)
    wait_for('no observations -> unknown', lambda: view().get('state') == 'unknown')
    # A foreign trust root cannot authenticate this Controller.
    generate(work/'foreign-pki', ())
    wrong = ssl.create_default_context(cafile=str(work/'foreign-pki/ca.crt'))
    try:
        with socket.create_connection(('127.0.0.1', rpc_port), timeout=3) as raw:
            with wrong.wrap_socket(raw, server_hostname='localhost'):
                raise AssertionError('foreign CA accepted')
    except ssl.SSLCertVerificationError:
        print('PASS: foreign CA rejected', flush=True)
    def start_agent(name):
        return start('netmonorc-agent-run-'+name, '-controller', f'127.0.0.1:{rpc_port}',
                     '-data-dir', str(work/name), '-allow-cidrs', '127.0.0.1/32',
                     '-ca', str(pki/'ca.crt'), '-cert', str(pki/f'{name}.crt'),
                     '-key', str(pki/f'{name}.key'),
                     *(['-blackbox-binary', args.blackbox_binary] if args.network else []))
    branch, dc = start_agent('branch'), start_agent('dc')
    wait_for('two authenticated processes -> healthy', lambda: view().get('state') == 'healthy')
    old = json.loads(sql("SELECT payload FROM nmo_observations WHERE agent_id='branch' ORDER BY finished_at LIMIT 1"))
    stop(controller)
    wait_for('observations buffered while Controller is offline',
             lambda: list((work/'branch').glob('*.json')), seconds=20)
    stop(branch)
    # Replay a previously committed record, modelling an ACK lost on the wire.
    replay = work/'branch'/(old['id']+'.json')
    replay.write_text(json.dumps(old)); replay.chmod(0o600)
    branch = start_agent('branch')
    time.sleep(1)
    assert replay.exists(), 'saved observation disappeared without a durable ACK'
    controller = start('netmonorc-controller', *controller_args)
    wait_for('Controller/Agent restart and durable replay', lambda: not replay.exists())
    assert sql("SELECT count(*) FROM nmo_observations WHERE id='"+old['id']+"'") == '1'
    print('PASS: replay did not create a duplicate database observation', flush=True)
    wait_for('assessment restored after reconnect', lambda: view().get('state') == 'healthy')
    Handler.status = 503
    wait_for('confirmed HTTP failures -> down', lambda: view().get('state') == 'down', seconds=25)
    Handler.status = 200
    wait_for('service recovery -> healthy', lambda: view().get('state') == 'healthy', seconds=25)
    stop(branch)
    wait_for('Agent loss plus TTL -> unknown, not down', lambda: view().get('state') == 'unknown', seconds=25)
    test_env = os.environ.copy()
    test_env['NETMONORC_TEST_DSN_FILE'] = str(dsn)
    subprocess.run([shutil.which('go'), 'test', './internal/storage', '-run',
                    'TestPostgresDurabilityAndFencing', '-count=1', '-v'],
                   cwd=ROOT, env=test_env, check=True)
    print('Evidence:', work, flush=True)
finally:
    for p in reversed(processes):
        stop(p)
    http.shutdown(); http.server_close(); thread.join()
    if dns:
        dns.close()
    if owned_container:
        command('docker', 'rm', '-fv', container)
    for f in files:
        f.close()
