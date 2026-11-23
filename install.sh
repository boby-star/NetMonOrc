#!/usr/bin/env bash
# Standalone NetMonOrc installer. Python is embedded to avoid eval/source of state.
set -Eeuo pipefail
umask 077
nmo_help=false
for arg in "$@"; do
  [[ "$arg" != --help && "$arg" != -h ]] || nmo_help=true
done
for arg in "$@"; do
  if [[ "$arg" == --install-deps && "$nmo_help" == false ]]; then
    [[ "$EUID" == 0 ]] || { echo 'Run as root (sudo bash install.sh ...).' >&2; exit 1; }
    # This explicit option changes packages only on the documented distribution.
    . /etc/os-release
    [[ "$ID" == ubuntu && "$VERSION_ID" == 24.04 ]] || { echo '--install-deps supports Ubuntu 24.04 only; install dependencies manually.' >&2; exit 1; }
    nmo_packages=(ca-certificates openssl python3)
    # Do not replace a pre-existing Docker CE/vendor installation with docker.io.
    if ! command -v docker >/dev/null; then
      nmo_packages+=(docker.io docker-buildx docker-compose-v2)
    fi
    apt-get update
    apt-get install -y "${nmo_packages[@]}"
    systemctl enable --now docker
    break
  fi
done
command -v python3 >/dev/null || { echo 'Python >=3.10 required (or --install-deps on Ubuntu 24.04).' >&2; exit 1; }
# Preserve the caller's TTY/pipe separately from the embedded Python source.
exec 3<&0
exec python3 - "$0" "$@" <<'PY'
import argparse, contextlib, fcntl, hashlib, http.client, ipaddress, json, os
from pathlib import Path
import re, secrets, shutil, socket, ssl, subprocess, sys, tempfile, time

SCRIPT = Path(sys.argv[1]).resolve()
parser = argparse.ArgumentParser(prog='install.sh', description='Install/manage one NetMonOrc master or Agent. Requires root, Linux, Python >=3.10, OpenSSL, Docker + Compose v2. Never removes data or volumes.')
parser.add_argument('role', choices=['master', 'agent'])
parser.add_argument('--action', choices=['install','update','status','logs','stop','admin'], default='install')
for flag in ['install-dir','instance','project','image','image-archive','build-ca','compose-override','source-dir','ca','cert','key','catalog','slots','grpc-bind','ui-bind','controller','server-name','allow-cidrs','webhook-allow-cidrs']:
    parser.add_argument('--'+flag)
parser.add_argument('--grpc-port', type=int)
parser.add_argument('--ui-port', type=int)
parser.add_argument('--timeout', type=int, default=90)
parser.add_argument('--username', default='admin')
for flag in ['install-deps','build','pull','rotate-tls','no-start','bootstrap-admin','password-stdin']:
    parser.add_argument('--'+flag, action='store_true')
a = parser.parse_args(sys.argv[2:])

def fail(message): raise RuntimeError(message)
def run(command, data=None, capture=True, stdin=None):
    r = subprocess.run(command, input=data, stdin=stdin, stdout=subprocess.PIPE if capture else None, stderr=subprocess.PIPE if capture else None)
    if r.returncode:
        # Credentials only travel in private files/stdin; never echo command inputs.
        detail = r.stderr.decode(errors='replace')[-2000:] if capture else ''
        fail('Command failed: '+str(command[0])+' '+detail)
    return r.stdout.decode() if capture else ''
def docker(*args, **kwargs): return run(['docker', *args], **kwargs)
def safe(value, name, pattern=r'[A-Za-z0-9_./:@,+=\[\]-]+'):
    if not value or not re.fullmatch(pattern,str(value)): fail('Invalid '+name+' (no whitespace, quotes or shell expansion)')
    return str(value)
def certificate(ca, cert, key, role, name=None):
    for path in [ca,cert,key]:
        if not path.is_file(): fail('TLS input unavailable: '+str(path))
    run(['openssl','verify','-CAfile',str(ca),'-purpose','sslserver' if role=='master' else 'sslclient',str(cert)])
    run(['openssl','x509','-in',str(cert),'-checkend','0','-noout'])
    pub = run(['openssl','x509','-in',str(cert),'-pubkey','-noout'])
    if pub != run(['openssl','pkey','-in',str(key),'-passin','pass:','-pubout']): fail('Certificate/private key mismatch')
    san = run(['openssl','x509','-in',str(cert),'-noout','-ext','subjectAltName'])
    if role=='agent':
        uris = re.findall(r'URI:([^,\s]+)',san)
        if len(uris)!=1 or not re.fullmatch(r'spiffe://netmonorc/agent/[a-z][a-z0-9-]{0,62}',uris[0]): fail('Exactly one Agent URI SAN spiffe://netmonorc/agent/<id> required')
        return uris[0].rsplit('/',1)[1]
    if not name:
        names=re.findall(r'DNS:([^,\s]+)',san)
        if not names: fail('Master requires --server-name matching its certificate DNS SAN')
        name=names[0]
    safe(name,'server-name',r'[A-Za-z0-9.-]+')
    run(['openssl','verify','-CAfile',str(ca),'-verify_hostname',name,str(cert)])
    return name

def atomic(path,data,uid=0,gid=0,mode=0o600):
    fd,tmp=tempfile.mkstemp(prefix='.install-',dir=path.parent)
    try:
        with os.fdopen(fd,'wb') as f:
            os.fchmod(f.fileno(),mode);os.fchown(f.fileno(),uid,gid);f.write(data);f.flush();os.fsync(f.fileno())
        os.replace(tmp,path)
    finally:
        if os.path.exists(tmp):os.unlink(tmp)

def tls_probe(host,port,name,ca,cert=None,key=None,is_http=False):
    ctx=ssl.create_default_context(cafile=str(ca));ctx.minimum_version=ssl.TLSVersion.TLSv1_3
    ctx.set_alpn_protocols(['http/1.1'] if is_http else ['h2'])
    if cert:ctx.load_cert_chain(str(cert),str(key))
    with socket.create_connection((host,port),timeout=4) as raw:
        with ctx.wrap_socket(raw,server_hostname=name) as s:
            if is_http:
                s.sendall(('GET /api/v1/auth/me HTTP/1.1\r\nHost: '+name+'\r\nConnection: close\r\n\r\n').encode())
                response=http.client.HTTPResponse(s);response.begin();return response.status in [200,401]
    return True

def compose_text(role):
    return MASTER_COMPOSE if role=='master' else AGENT_COMPOSE

MASTER_COMPOSE = r'''# Independent Controller host. Paths in the env file must be absolute.
name: netmonorc-server
services:
  postgres:
    image: postgres:17.6-alpine@sha256:ef257d85f76e48da1c64832459b59fcaba1a4dac97bf5d7450c77753542eee94
    environment:
      POSTGRES_USER: netmonorc
      POSTGRES_DB: netmonorc
      POSTGRES_PASSWORD_FILE: /run/secrets/postgres-password
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ${NMO_CONFIG_DIR:?absolute config directory required}/postgres-password:/run/secrets/postgres-password:ro
    networks: [database]
    healthcheck:
      test: [CMD, pg_isready, -h, 127.0.0.1, -U, netmonorc, -d, netmonorc]
      interval: 2s
      timeout: 2s
      retries: 30
    restart: unless-stopped
  controller:
    image: ${NMO_IMAGE:-netmonorc-research:local}
    command:
      - -config=/etc/netmonorc/catalog.yaml
      - -database-file=/etc/netmonorc/database.dsn
      - -slots=${NMO_SLOTS:?agent=zone roster required}
      - -listen=0.0.0.0:9443
      - -ca=/etc/netmonorc/ca.crt
      - -cert=/etc/netmonorc/controller.crt
      - -key=/etc/netmonorc/controller.key
      - -assessment-file=/data/assessment.json
      - -diagnostic-file=/data/diagnostic.json
      - -alert-file=/data/alert.json
      - -http-listen=0.0.0.0:8443
      - -http-ui-dir=/usr/local/share/netmonorc/web
      - -webhook-allow-cidrs=${NMO_WEBHOOK_ALLOW_CIDRS:-}
    ports:
      - ${NMO_GRPC_BIND:-127.0.0.1}:${NMO_GRPC_PORT:-9443}:9443
      - ${NMO_UI_BIND:-127.0.0.1}:${NMO_UI_PORT:-8443}:8443
    volumes:
      - ${NMO_CONFIG_DIR:?absolute config directory required}:/etc/netmonorc:ro
      - ${NMO_DATA_DIR:?absolute data directory required}:/data
    networks: [database, control]
    depends_on:
      postgres: {condition: service_healthy}
    read_only: true
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    tmpfs: ['/tmp:rw,noexec,nosuid,size=64m,mode=0700,uid=10001,gid=10001']
    restart: unless-stopped
    pids_limit: 128
volumes:
  postgres-data:
networks:
  database: {internal: true}
  control:
'''
AGENT_COMPOSE = r'''# Install on each Agent host; the Agent opens an outgoing mTLS connection.
name: netmonorc-agent
services:
  agent:
    image: ${NMO_IMAGE:-netmonorc-research:local}
    entrypoint: [/usr/local/bin/netmonorc-agent]
    command:
      - -controller=${NMO_CONTROLLER:?Controller host:port required}
      - -server-name=${NMO_SERVER_NAME:?Controller certificate DNS name required}
      - -ca=/etc/netmonorc/ca.crt
      - -cert=/etc/netmonorc/agent.crt
      - -key=/etc/netmonorc/agent.key
      - -data-dir=/data
      - -allow-cidrs=${NMO_ALLOW_CIDRS:?explicit target CIDRs required}
      - -blackbox-binary=/usr/local/bin/blackbox_exporter
    volumes:
      - ${NMO_CONFIG_DIR:?absolute config directory required}:/etc/netmonorc:ro
      - ${NMO_DATA_DIR:?absolute data directory required}:/data
    read_only: true
    cap_drop: [ALL]
    security_opt: [no-new-privileges:true]
    tmpfs: ['/tmp:rw,noexec,nosuid,size=64m,mode=0700,uid=10001,gid=10001']
    restart: unless-stopped
    pids_limit: 128
'''

def main():
    if sys.version_info < (3,10): fail('Python >=3.10 required')
    if os.geteuid()!=0: fail('Run as root: sudo bash install.sh ...')
    if sys.platform!='linux':fail('This installer supports Linux only')
    if not 1<=a.timeout<=600:fail('--timeout must be 1..600 seconds')
    for command in ['openssl','docker']:
        if not shutil.which(command):fail(command+' required; use --install-deps on Ubuntu 24.04 or install manually')
    docker('info','--format','{{.ServerVersion}}');docker('compose','version')
    endpoint=docker('context','inspect','--format','{{.Endpoints.docker.Host}}').strip() if os.environ.get('DOCKER_CONTEXT') else (os.environ.get('DOCKER_HOST') or docker('context','inspect','--format','{{.Endpoints.docker.Host}}').strip())
    if not endpoint.startswith('unix://'):fail('Run installer on the Docker host with a local Unix socket; remote contexts are unsupported')
    if 'rootless' in docker('info','--format','{{json .SecurityOptions}}'):fail('This installer requires rootful Docker for UID 10001 bind mounts')
    if a.role=='agent' and (a.slots or a.catalog or a.grpc_bind or a.ui_bind or a.grpc_port or a.ui_port or a.webhook_allow_cidrs or a.bootstrap_admin or a.action=='admin'):fail('Master-only option supplied for Agent')
    if a.role=='master' and (a.controller or a.allow_cidrs):fail('Agent-only option supplied for master')
    if a.role=='master' and a.instance not in [None,'master']:fail('--instance is the Agent identity; use --install-dir/--project for separate masters')
    supplied_tls=[a.ca,a.cert,a.key]
    if any(supplied_tls) and not all(supplied_tls):fail('--ca, --cert and --key must be supplied together')
    tls_paths=[Path(x).resolve() for x in supplied_tls] if all(supplied_tls) else None
    tls_identity=certificate(*tls_paths,a.role,a.server_name) if tls_paths else None
    instance=a.instance or ('master' if a.role=='master' else tls_identity)
    if not instance and not a.install_dir:fail('Agent requires TLS inputs on first install; use --instance <id> or --install-dir for later management')
    if instance:safe(instance,'instance',r'[a-z][a-z0-9-]{0,62}')
    dest=Path(a.install_dir or ('/srv/netmonorc/'+('master' if a.role=='master' else 'agent-'+instance)))
    safe(str(dest),'install-dir',r'/[A-Za-z0-9_./-]+')
    if '..' in dest.parts or dest.is_symlink():fail('install-dir must not contain parent traversal or be a symlink')
    dest=dest.absolute()
    if dest in [Path('/'),Path('/srv'),Path('/srv/netmonorc')]:fail('Choose a dedicated installation directory')
    if a.action not in ['install','update'] and not dest.is_dir():fail('Installation not found: '+str(dest))
    dest.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
    lock_path=dest.parent/('.'+dest.name+'.install.lock')
    fd=os.open(lock_path,os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
    with os.fdopen(fd,'w') as lock:
        try:fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except BlockingIOError:fail('Another installer is managing this directory')
        state_path=dest/'installation.json';old=None
        if dest.exists():
            if not state_path.is_file() or state_path.is_symlink():fail('Existing unmanaged directory preserved; choose a new --install-dir')
            if state_path.stat().st_uid!=0 or state_path.stat().st_mode&0o022:fail('Installer state must be root-owned and not writable by others')
            old=json.loads(state_path.read_text())
            if old.get('version')!=1 or old.get('role')!=a.role:fail('Installation role/version mismatch')
            if instance and instance!=old['instance']:fail('Installation identity cannot change')
            instance=old['instance']
        if a.action=='update' and not old:fail('Update requires an existing managed installation')
        default_project='netmonorc-'+('master' if a.role=='master' else 'agent-'+instance)
        if len(default_project)>63:default_project=default_project[:50]+'-'+hashlib.sha256(instance.encode()).hexdigest()[:12]
        project=a.project or (old['project'] if old else default_project)
        safe(project,'project',r'[a-z0-9][a-z0-9_-]{0,62}')
        if old and project!=old['project']:fail('Project cannot change: it owns persistent volumes')
        def compose(*args,**kwargs):
            files=['-f',str(dest/'compose.yaml')]
            if (dest/'compose.override.yaml').is_file():files+=['-f',str(dest/'compose.override.yaml')]
            return docker('compose','-p',project,'--env-file',str(dest/'compose.env'),*files,*args,**kwargs)
        def admin():
            if not a.password_stdin and not os.isatty(3):fail('Admin bootstrap requires a TTY or --password-stdin through a protected pipe')
            safe(a.username,'username',r'[A-Za-z0-9_.-]+')
            command=['exec']+(['-T'] if a.password_stdin else [])+['controller','/usr/local/bin/netmonorc-admin','-database-file','/etc/netmonorc/database.dsn','-username',a.username]+(['-password-stdin'] if a.password_stdin else [])
            compose(*command,capture=False,stdin=3)
        if a.action in ['status','logs','stop','admin']:
            if not old:fail('Managed installation required')
            if a.action=='admin':admin()
            else:compose(*({'status':['ps','--all'],'logs':['logs','--tail','100'],'stop':['stop']}[a.action]),capture=False)
            return
        if a.password_stdin and not a.bootstrap_admin:fail('--password-stdin on install/update requires --bootstrap-admin')
        if a.bootstrap_admin and a.no_start:fail('--bootstrap-admin requires starting master')
        if a.bootstrap_admin and not a.password_stdin and not os.isatty(3):fail('Use a TTY or --bootstrap-admin --password-stdin')
        if not old:
            if not tls_paths:fail('First installation requires --ca --cert --key')
            for kind in ['ps','volume']:
                args=['ps','-aq'] if kind=='ps' else ['volume','ls','-q']
                if docker(*args,'--filter','label=com.docker.compose.project='+project).strip():fail('Existing Compose project/volumes preserved; choose a new --project')
        values=dict(old['env']) if old else {'NMO_IMAGE':'netmonorc:research','NMO_CONFIG_DIR':str(dest/'config'),'NMO_DATA_DIR':str(dest/'data')}
        flags={'image':'NMO_IMAGE','slots':'NMO_SLOTS','grpc_bind':'NMO_GRPC_BIND','grpc_port':'NMO_GRPC_PORT','ui_bind':'NMO_UI_BIND','ui_port':'NMO_UI_PORT','controller':'NMO_CONTROLLER','server_name':'NMO_SERVER_NAME','allow_cidrs':'NMO_ALLOW_CIDRS','webhook_allow_cidrs':'NMO_WEBHOOK_ALLOW_CIDRS'}
        if a.role=='master':
            for k,v in {'NMO_GRPC_BIND':'127.0.0.1','NMO_UI_BIND':'127.0.0.1','NMO_GRPC_PORT':'9443','NMO_UI_PORT':'8443'}.items():values.setdefault(k,v)
        for flag,key in flags.items():
            v=getattr(a,flag)
            if v is not None:values[key]=str(v)
        for k,v in values.items():
            if k!='NMO_WEBHOOK_ALLOW_CIDRS' or v:safe(v,k)
        for k in ['NMO_GRPC_PORT','NMO_UI_PORT']:
            if k in values and not 1<=int(values[k])<=65535:fail('Invalid port')
        if a.role=='master':
            slots=values.get('NMO_SLOTS','').split(',');ids=[]
            for item in slots:
                if not re.fullmatch(r'[a-z][a-z0-9-]{0,62}=[a-z][a-z0-9-]{0,62}',item):fail('--slots requires agent=zone pairs')
                ids.append(item.split('=')[0])
            if not 1<=len(ids)<=100 or len(set(ids))!=len(ids):fail('Roster requires 1..100 distinct Agent identities')
            for k in ['NMO_GRPC_BIND','NMO_UI_BIND']:
                if ipaddress.ip_address(values[k]).version!=4:fail('Published bind addresses in this installer must be IPv4')
            server_name=a.server_name or (old['serverName'] if old else tls_identity)
        else:
            if not values.get('NMO_CONTROLLER') or not values.get('NMO_SERVER_NAME') or not values.get('NMO_ALLOW_CIDRS'):fail('Agent requires --controller host:port --server-name DNS --allow-cidrs CIDR[,CIDR]')
            server_name=values['NMO_SERVER_NAME']
        safe(server_name,'server-name',r'[A-Za-z0-9.-]+')
        for k in ['NMO_ALLOW_CIDRS','NMO_WEBHOOK_ALLOW_CIDRS']:
            if values.get(k):
                for cidr in values[k].split(','):ipaddress.ip_network(cidr,strict=False)
        if old and (values['NMO_CONFIG_DIR']!=str(dest/'config') or values['NMO_DATA_DIR']!=str(dest/'data')):fail('Managed paths cannot change')
        ca,cert,key=tls_paths or [dest/'config'/f for f in ['ca.crt', 'controller.crt' if a.role=='master' else 'agent.crt','controller.key' if a.role=='master' else 'agent.key']]
        identity=certificate(ca,cert,key,a.role,server_name)
        if a.role=='agent' and identity!=instance:fail('Certificate identity must equal --instance; existing spool identity cannot change')
        tls_bytes=[p.read_bytes() for p in [ca,cert,key]]
        target_names=['ca.crt','controller.crt','controller.key'] if a.role=='master' else ['ca.crt','agent.crt','agent.key']
        tls_changed=old and any((dest/'config'/name).read_bytes()!=data for name,data in zip(target_names,tls_bytes))
        if tls_changed and not a.rotate_tls:fail('TLS changes require --rotate-tls; existing files preserved')
        if a.role=='master':
            if not old and not a.catalog:fail('Master requires --catalog YAML')
            catalog=Path(a.catalog).read_bytes() if a.catalog else (dest/'config/catalog.yaml').read_bytes()
            if old and catalog!=(dest/'config/catalog.yaml').read_bytes():fail('Bootstrap catalog is immutable here; change resources through API/CLI/React')
            if old:
                password=(dest/'config/postgres-password').read_text().strip()
                if (dest/'config/database.dsn').read_text().strip()!=f'postgresql://netmonorc:{password}@postgres:5432/netmonorc?sslmode=disable':fail('Existing database credentials inconsistent; preserved for manual recovery')
        image=values['NMO_IMAGE']
        if a.image_archive:docker('load','-i',str(Path(a.image_archive).resolve()))
        if a.build:
            source=Path(a.source_dir).resolve() if a.source_dir else SCRIPT.parent
            if not (source/'Dockerfile').is_file():fail('--build needs a checkout with Dockerfile; supply --source-dir')
            docker('buildx','version');print('Building '+image,flush=True)
            build_args=['--secret','id=build_ca,src='+str(Path(a.build_ca).resolve())] if a.build_ca else []
            docker('build',*build_args,'-t',image,str(source),capture=False)
        elif a.build_ca:fail('--build-ca requires --build')
        elif a.pull:docker('pull',image,capture=False)
        elif subprocess.run(['docker','image','inspect',image],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode:docker('pull',image,capture=False)
        if a.role=='master':
            result=json.loads(docker('run','--rm','-i','--network','none','--entrypoint','/usr/local/bin/netmonorc',image,'validate','--config','-','--slots',values['NMO_SLOTS'],data=catalog))
            if not result.get('valid'):fail('Catalog validation failed')
        else:
            address=values['NMO_CONTROLLER']
            if address.startswith('['):match=re.fullmatch(r'\[([^]]+)\]:(\d+)',address)
            else:match=re.fullmatch(r'([^:]+):(\d+)',address)
            if not match or not 1<=int(match[2])<=65535:fail('--controller requires host:port (IPv6: [address]:port)')
            if not a.no_start:tls_probe(match[1],int(match[2]),server_name,ca,cert,key)
        state={'version':1,'role':a.role,'instance':instance,'project':project,'serverName':server_name,'env':values}
        env=''.join(k+'='+v+'\n' for k,v in sorted(values.items())).encode()
        contents={'compose.env':env,'compose.yaml':compose_text(a.role).encode(),'installation.json':(json.dumps(state,indent=2)+'\n').encode(),'install.sh':SCRIPT.read_bytes()}
        if a.compose_override:
            contents['compose.override.yaml']=Path(a.compose_override).read_bytes()
        # Validate the complete desired Compose before touching managed state.
        with tempfile.TemporaryDirectory(prefix='nmo-preflight-') as scratch:
            scratch=Path(scratch);(scratch/'compose.env').write_bytes(env);(scratch/'compose.yaml').write_bytes(contents['compose.yaml'])
            files=['-f',str(scratch/'compose.yaml')]
            override=contents.get('compose.override.yaml')
            if override is None and old and (dest/'compose.override.yaml').is_file():override=(dest/'compose.override.yaml').read_bytes()
            if override is not None:
                (scratch/'compose.override.yaml').write_bytes(override);files+=['-f',str(scratch/'compose.override.yaml')]
            docker('compose','-p',project,'--env-file',str(scratch/'compose.env'),*files,'config','--quiet')
        if old:
            changed=bool(tls_changed) or any(not (dest/f).is_file() or (dest/f).read_bytes()!=v for f,v in contents.items())
            if changed:
                backup=dest/'history'/(time.strftime('%Y%m%dT%H%M%SZ',time.gmtime())+'-'+secrets.token_hex(3));backup.mkdir(parents=True,mode=0o700)
                for f in set(contents)|{'compose.override.yaml'}:
                    if (dest/f).is_file():shutil.copy2(dest/f,backup/f)
                shutil.copytree(dest/'config',backup/'config')
                for p in backup.rglob('*'):
                    if p.is_dir():p.chmod(0o700)
                    else:p.chmod(0o600)
                for f,v in contents.items():atomic(dest/f,v,mode=0o700 if f=='install.sh' else 0o600)
                for f,v in zip(target_names,tls_bytes):atomic(dest/'config'/f,v,10001,10001)
        else:
            stage=Path(tempfile.mkdtemp(prefix='.'+dest.name+'-',dir=dest.parent))
            try:
                for name in ['config','data']:
                    (stage/name).mkdir(mode=0o700);os.chown(stage/name,10001,10001)
                for f,v in contents.items():atomic(stage/f,v,mode=0o700 if f=='install.sh' else 0o600)
                for f,v in zip(target_names,tls_bytes):atomic(stage/'config'/f,v,10001,10001)
                if a.role=='master':
                    password=secrets.token_urlsafe(32)
                    for f,v in {'catalog.yaml':catalog,'postgres-password':(password+'\n').encode(),'database.dsn':f'postgresql://netmonorc:{password}@postgres:5432/netmonorc?sslmode=disable\n'.encode()}.items():atomic(stage/'config'/f,v,10001,10001)
                os.rename(stage,dest)
            finally:
                if stage.exists():shutil.rmtree(stage)
        compose('config','--quiet')
        print('Configuration ready: '+str(dest)+'; project '+project,flush=True)
        if a.no_start:print('Prepared only (--no-start). No services started.');return
        if a.role=='master':compose('up','-d','--wait','--wait-timeout',str(a.timeout),'postgres',capture=False)
        service='controller' if a.role=='master' else 'agent'
        compose('up','-d',*(['--force-recreate'] if tls_changed else []),service,capture=False)
        if a.role=='master':
            host=values['NMO_UI_BIND'];host='127.0.0.1' if host=='0.0.0.0' else host
            until=time.monotonic()+a.timeout
            while True:
                try:
                    if tls_probe(host,int(values['NMO_UI_PORT']),server_name,dest/'config/ca.crt',is_http=True):break
                except (OSError,ssl.SSLError,http.client.HTTPException):pass
                if time.monotonic()>until:fail('HTTPS readiness timed out; configuration/data preserved. Use --action logs/status')
                time.sleep(1)
            print('Master HTTPS is ready. Agent coverage is a separate check in the UI.')
            if a.bootstrap_admin:admin()
            else:print('Create initial admin: sudo bash '+str(dest/'install.sh')+' master --install-dir '+str(dest)+' --action admin')
        else:
            cid=compose('ps','--all','--quiet','agent').strip();time.sleep(2)
            running=docker('inspect','--format','{{.State.Running}}',cid).strip()
            if running!='true':fail('Agent exited; inspect --action logs. Data preserved')
            print('Agent started; TLS endpoint verified. Confirm online/assignment/coverage on Controller.')
        print('Persistent directories and PostgreSQL volumes preserved; no prune/reset performed.')

try:main()
except (RuntimeError,ValueError,OSError,KeyError,TypeError) as e:
    print('Installation failed: '+str(e),file=sys.stderr);sys.exit(1)
PY
