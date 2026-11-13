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
import sys
import urllib.request
import urllib.error
from html.parser import HTMLParser
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
parser.add_argument('--cel', action='store_true')
parser.add_argument('--extract', action='store_true')
parser.add_argument('--diagnostics', choices=['adaptive','fixed','static'])
parser.add_argument('--diagnostic-runner',choices=['native-http','blackbox'],default='native-http')
parser.add_argument('--blackbox-binary')
parser.add_argument('--diagnostic-retry',action='store_true',help='research fixture: fail exactly one diagnostic exporter startup')
parser.add_argument("--http-ui",action="store_true",help="verify real HTTPS assessment REST and built React assets")
args = parser.parse_args()
if sum([bool(args.network),bool(args.cel),bool(args.extract),bool(args.diagnostics)])>1:
    parser.error('choose one fixture mode at a time')
if args.diagnostic_runner=='blackbox' and (not args.diagnostics or not args.blackbox_binary):
    parser.error('Blackbox diagnostics require --diagnostics and --blackbox-binary')
if args.network and not args.blackbox_binary:
    parser.error('--network requires --blackbox-binary')
if args.diagnostic_retry and (not args.diagnostics or args.diagnostic_runner!='blackbox' or not args.blackbox_binary):
    parser.error('--diagnostic-retry requires Blackbox diagnostics and its prepared binary')
dns = DNSFixture() if args.network else None
processes = []
files = []
owned_container = False
retry_report = None
retry_tasks = None

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
    omit_ready = False
    diagnostic_delay = 0
    def handle(self):
        try:
            super().handle()
        except (BrokenPipeError, ConnectionResetError):
            pass  # Expected when a bounded diagnostic is cancelled.
    def do_GET(self):
        if self.path=='/diagnostic' and self.diagnostic_delay:
            time.sleep(self.diagnostic_delay)
        self.send_response(200 if args.extract else self.status)
        self.end_headers()
        if args.extract:
            ready = '' if self.omit_ready else (' ready=true' if self.status == 200 else ' ready=false')
            self.wfile.write(('queue_depth=0 ratio=1.0 serial=9007199254740993'+ready).encode())
        else:
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
    example = 'network.yaml' if args.network else ('extract.yaml' if args.extract else ('cel.yaml' if args.cel else 'http.yaml'))
    text = (ROOT/'examples'/example).read_text().replace('18080', str(http.server_port))
    if dns:
        text = text.replace('1053', str(dns.port))
    text = text.replace('interval: 30s', 'interval: 10s').replace('maxAge: 90s', 'maxAge: 20s')
    if args.diagnostics:
        text += '\n  diagnostics:\n    enabled: true\n    profileRef: live-web\n    cooldown: 1s\n'
        profile = work/'diagnostic-profile.yaml'
        profile.write_text((ROOT/'examples/diagnostics'/('live-'+('adaptive' if args.diagnostics=='static' else args.diagnostics)+'.yaml')).read_text().replace('18080',str(http.server_port)).replace("/', expectedStatus","/diagnostic', expectedStatus").replace("runner: native-http","runner: "+args.diagnostic_runner))
        if args.diagnostic_retry:
            profile.write_text(profile.read_text().replace('maxAdditionalChecks: 2','maxAdditionalChecks: 3').replace('    - id: inspect-server\n','    - id: inspect-server\n      retry: {maxAttempts: 2}\n'))
            # Operator-selected test executable, outside manifests and product
            # code. It exits once before exporter readiness; all other launches
            # exec the real prepared Blackbox binary without a shell.
            wrapper = work/'blackbox-once.py'
            marker = work/'startup-failed-once'
            binary = str(Path(args.blackbox_binary).resolve())
            wrapper.write_text('#!'+sys.executable+'\n'+
                'import json,os,sys\n'+
                'from pathlib import Path\n'+
                'path=next(a.split("=",1)[1] for a in sys.argv[1:] if a.startswith("--config.file="))\n'+
                'modules=json.loads(Path(path).read_text())["modules"]\n'+
                'if "inspect-server" in modules:\n'+
                '    try:\n'+
                '        fd=os.open('+repr(str(marker))+',os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600);os.close(fd)\n'+
                '    except FileExistsError: pass\n'+
                '    else: sys.exit(7)\n'+
                'os.execv('+repr(binary)+',['+repr(binary)+',*sys.argv[1:]])\n')
            wrapper.chmod(0o700)
            args.blackbox_binary=str(wrapper)
    manifest.write_text(text)
    pki = work/'pki'
    controller_args = ['-config', str(manifest), '-listen', f'127.0.0.1:{rpc_port}',
                       '-database-file', str(dsn), '-slots', 'branch=office,dc=server',
                       '-ca', str(pki/'ca.crt'), '-cert', str(pki/'controller.crt'),
                       '-key', str(pki/'controller.key'), '-assessment-file', str(work/'assessment.json'),
                       '-report-file',str(work/'report.html')]
    if args.http_ui:
        with socket.socket() as sock:
            sock.bind(('127.0.0.1',0));api_port=sock.getsockname()[1]
        viewer_token=secrets.token_urlsafe(32)
        token_file=work/'viewer.token';token_file.write_text(viewer_token);token_file.chmod(0o600)
        controller_args += ['-http-listen',f'127.0.0.1:{api_port}','-http-token-file',str(token_file),'-http-ui-dir',str(ROOT/'web/dist')]
    if args.diagnostics:
        controller_args += ['-diagnostic-profile',str(profile),'-diagnostic-file',str(work/'diagnostic.json')]
        if args.diagnostics=='static':
            controller_args += ['-diagnostic-comparison-static']
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
                     *(['-blackbox-binary', args.blackbox_binary] if args.blackbox_binary else []))
    branch, dc = start_agent('branch'), start_agent('dc')
    wait_for('two authenticated processes -> healthy', lambda: view().get('state') == 'healthy')
    wait_for('private HTML report reflects live assessment',lambda: (work/'report.html').exists() and 'Працює' in (work/'report.html').read_text())
    assert (work/'report.html').stat().st_mode & 0o077==0
    if args.http_ui:
        verified_tls=ssl.create_default_context(cafile=str(pki/'ca.crt'))
        base=f'https://localhost:{api_port}'
        def get(path,authorized=True):
            request=urllib.request.Request(base+path,headers={'Authorization':'Bearer '+viewer_token} if authorized else {})
            with urllib.request.urlopen(request,context=verified_tls,timeout=5) as response:return response.read()
        dto=json.loads(get('/api/v1/monitors/research-web/assessment'))
        assert dto['monitorId']=='research-web' and dto['state']=='healthy' and len(dto['zoneStates'])==2,dto
        assert dto['coverageState']=='sufficient' and dto['evidenceIds']
        try:get('/api/v1/monitors/research-web/assessment',False);raise AssertionError('anonymous API access')
        except urllib.error.HTTPError as error:
            assert error.code==401
            assert json.loads(error.read())['error']['requestId']
        class Assets(HTMLParser):
            def __init__(self):super().__init__();self.paths=[]
            def handle_starttag(self,tag,attrs):
                for key,value in attrs:
                    if key in ['src','href'] and value.startswith('/assets/'):self.paths.append(value)
        assets=Assets();assets.feed(get('/').decode())
        assert assets.paths,'React build not served'
        for asset in assets.paths:assert len(get(asset))>0
        assert 'NetMonOrc' in get('/').decode()
        print('PASS: trusted HTTPS REST returns live PostgreSQL Assessment; anonymous rejected; production React assets served',flush=True)
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
    wait_for('confirmed service failure -> down', lambda: view().get('state') == 'down', seconds=25)
    if args.cel:
        assert all(z['reason'] == 'HTTP_SERVER_ERROR' for z in view()['zones'])
        assert int(sql("SELECT count(*) FROM nmo_observations WHERE payload->>'statusCode'='503' AND payload->>'outcome'='success'")) >= 2
        print('PASS: CEL classifies accepted HTTP 503 observations as resource down', flush=True)
    if args.extract:
        assert all(z['reason'] == 'APPLICATION_NOT_READY' for z in view()['zones'])
        stored = json.loads(sql("SELECT payload FROM nmo_observations WHERE payload->'values'->>'ready'='false' LIMIT 1"))
        assert stored['statusCode'] == 200 and stored['outcome'] == 'success'
        assert stored['values']['queue_depth'] == 0 and stored['values']['ready'] is False
        assert stored['values']['serial'] == 9007199254740993 and isinstance(stored['values']['ratio'], float)
        print('PASS: typed values survived PostgreSQL; HTTP 200 + ready=false -> resource down', flush=True)
    if args.diagnostics:
        def diagnosis():
            try:
                return json.loads((work/'diagnostic.json').read_text())
            except (FileNotFoundError,json.JSONDecodeError):
                return None
        report = wait_for('real Agent diagnostics -> supported service-failure',
                          lambda: (r if (r:=diagnosis()) and r['result']['status']=='completed' else None),seconds=25)
        assert report['result']['mode']==args.diagnostics,report
        assert report['result']['hypotheses']==['service-failure'], report
        expected = (1 if args.diagnostics=='adaptive' else 2)+(1 if args.diagnostic_retry else 0)
        assert report['result']['attempts']==expected, report
        assert int(sql("SELECT count(*) FROM nmo_diagnostic_tasks WHERE state='finished'"))==expected
        assert int(sql("SELECT count(*) FROM nmo_observations WHERE diagnostic_task_id<>''"))==expected
        if args.diagnostic_retry:
            retry_report=report
            retry_tasks=json.loads(sql("SELECT json_agg(json_build_object('id',t.id,'check',t.check_id,'attemptNo',t.attempt_no,'state',t.state,'observation',o.payload)) FROM nmo_diagnostic_tasks t LEFT JOIN nmo_observations o ON t.id=o.diagnostic_task_id WHERE t.run_id='"+report['id']+"'"))
            technical=[t for t in retry_tasks if t['observation']['execution']=='execution_failed']
            assert len(technical)==1 and technical[0]['check']=='inspect-server' and technical[0]['attemptNo']==1,retry_tasks
            second=[t for t in retry_tasks if t['check']=='inspect-server' and t['attemptNo']==2]
            assert len(second)==1 and second[0]['observation']['execution']=='completed' and second[0]['observation']['outcome']=='failure',retry_tasks
            assert technical[0]['id']!=second[0]['id'],'retry reused physical task identity'
            assert report['reservedChecks']==expected
            print('PASS: one real exporter startup failure retried with a new task ID and durable attempt 2',flush=True)
        first_run = report['id']
        # Unchanged failure and a Controller restart cannot renew this episode.
        stop(controller)
        controller = start('netmonorc-controller', *controller_args)
        wait_for('same failure assessed after diagnostic Controller restart',lambda: view().get('state')=='down')
        time.sleep(3)
        assert sql("SELECT count(*) FROM nmo_diagnostic_runs")=='1'
        assert int(sql("SELECT used_checks FROM nmo_diagnostic_runs WHERE id='"+first_run+"'"))==expected
        print('PASS: physical probes committed; reconnect/restart did not reset episode budget',flush=True)
    Handler.status = 200
    wait_for('service recovery -> healthy', lambda: view().get('state') == 'healthy', seconds=25)
    if args.diagnostics:
        wait_for('healthy/full coverage closes incident',lambda: sql("SELECT count(*) FROM nmo_incidents WHERE episode_id<>''")=='0')
        Handler.status=503
        Handler.diagnostic_delay=8
        wait_for('new incident admitted with an active physical task',
                 lambda: sql("SELECT count(*) FROM nmo_diagnostic_tasks t JOIN nmo_diagnostic_runs r ON r.id=t.run_id WHERE r.state='running' AND t.state='accepted'")=='1',seconds=25)
        controller.kill();controller.wait()
        controller=start('netmonorc-controller',*controller_args)
        wait_for('crashed run remains inconclusive with preserved reservation',
                 lambda: sql("SELECT count(*) FROM nmo_diagnostic_runs WHERE reason='controller_restart' AND used_checks=1 AND state='inconclusive'")=='1')
        restored=wait_for('interrupted report restored from PostgreSQL',
                          lambda: (r if (r:=diagnosis()) and r['id']!=first_run and r['result']['reason']=='controller_restart' else None))
        assert restored['reservedChecks']==1
        time.sleep(3)
        assert sql("SELECT count(*) FROM nmo_diagnostic_runs")=='2'
        assert int(sql("SELECT count(*) FROM nmo_diagnostic_tasks"))==expected+1
        Handler.diagnostic_delay=0
        Handler.status=200
        wait_for('recovery after interrupted diagnostic',lambda: view().get('state')=='healthy',seconds=25)
    if args.extract:
        Handler.omit_ready = True
        wait_for('missing extracted field -> unknown without fabricated false/zero', lambda: view().get('state') == 'unknown', seconds=25)
        assert all(z['coverage'] == 'complete' for z in view()['zones'])
        Handler.omit_ready = False
        wait_for('extracted field restored -> healthy', lambda: view().get('state') == 'healthy', seconds=25)
    stop(branch)
    wait_for('Agent loss plus TTL -> unknown, not down', lambda: view().get('state') == 'unknown', seconds=25)
    test_env = os.environ.copy()
    test_env['NETMONORC_TEST_DSN_FILE'] = str(dsn)
    subprocess.run([shutil.which('go'), 'test', './internal/storage', '-run',
                    'TestPostgres', '-count=1', '-v'],
                   cwd=ROOT, env=test_env, check=True)
    result={'status': 'passed', 'fixture': ('diagnostics-'+args.diagnostics) if args.diagnostics else ('extract' if args.extract else ('cel' if args.cel else ('network' if args.network else 'http'))), 'postgres_fencing_test': 'passed', 'replay': 'passed', 'ttl': 'passed'}
    if args.http_ui: result['httpUi']={'httpsVerified':True,'assessmentContractVerified':True,'anonymousRejected':True,'reactAssetsServed':True,'browserInteractionTested':False}
    if args.diagnostic_retry: result.update({'diagnosticRetry':True,'diagnostic':retry_report,'tasks':retry_tasks})
    (work/'result.json').write_text(json.dumps(result,indent=2)+'\n')
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
