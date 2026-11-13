"""Manage only the named, private NetMonOrc Compose research stand."""
import argparse
import hashlib
import io
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import tarfile
import time
from dev_pki import generate, issue_server

ROOT = Path(__file__).resolve().parents[1]
VIP = '198.18.0.80'
NORMAL = {'http_status':200,'dns_mode':'answers','tcp_open':True}
SCENARIOS = {
    'healthy': NORMAL,
    'http': {**NORMAL,'http_status':503},
    'dns': {**NORMAL,'dns_mode':'nxdomain'},
    'dns-drop': {**NORMAL,'dns_mode':'drop'},
    'tcp': {**NORMAL,'tcp_open':False},
    'office-path': NORMAL,
    'multiple': {**NORMAL,'http_status':503,'dns_mode':'nxdomain'},
    'http-tcp': {**NORMAL,'http_status':503,'tcp_open':False},
    'dns-tcp': {**NORMAL,'dns_mode':'nxdomain','tcp_open':False},
    'all-protocols': {**NORMAL,'http_status':503,'dns_mode':'nxdomain','tcp_open':False},
}

def manifests():
    return {name:(ROOT/'deployment'/'lab'/name).read_text()
            for name in ['monitor.yaml','adaptive.yaml','fixed.yaml','adaptive-confirmed.yaml','fixed-confirmed.yaml','alert.yaml']}

class Lab:
    def __init__(self, project='netmonorc-lab', prefix='172.30'):
        if not re.fullmatch('[a-z][a-z0-9-]{0,39}',project):
            raise ValueError('project must be a lowercase Docker project name (max 40 chars)')
        network=ipaddress.IPv4Network(prefix+'.0.0/16')
        if not any(network.subnet_of(ipaddress.IPv4Network(n)) for n in ['10.0.0.0/8','172.16.0.0/12','192.168.0.0/16']):
            raise ValueError('stand prefix must be inside an RFC1918 private network')
        if not re.fullmatch(r'\d{1,3}\.\d{1,3}',prefix):
            raise ValueError('prefix must contain exactly two IPv4 octets')
        self.project,self.prefix = project,prefix
        self.work = ROOT/'.local'/'lab'/project
        self.env = {**os.environ,'NMO_NET_PREFIX':prefix}
    def compose(self,*args,input=None,env=None):
        return subprocess.run(['docker','compose','-p',self.project,'-f',str(ROOT/'compose.yaml'),*args],
                              cwd=ROOT,env={**self.env,**(env or {})},input=input,
                              stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True).stdout
    def require(self):
        metadata = json.loads((self.work/'prepared.json').read_text())
        if metadata!={'project':self.project,'prefix':self.prefix}:
            raise RuntimeError('stand ownership/prefix differs from the saved preparation')
    def prepare(self):
        self.work.mkdir(parents=True,mode=0o700,exist_ok=True)
        if self.compose('ps','--status','running','--quiet').strip():
            raise RuntimeError('stop this stand before preparing its configuration volumes')
        marker = self.work/'prepared.json'
        if not marker.exists():
            existing=subprocess.check_output(['docker','volume','ls','--filter','label=com.docker.compose.project='+self.project,'--quiet'])
            if existing.strip():raise RuntimeError('existing project volumes have no local ownership marker; recover their configuration before preparing')
        if marker.exists():
            self.require()
        else:
            generate(self.work/'pki')
            (self.work/'password').write_text(secrets.token_urlsafe(32))
            (self.work/'password').chmod(0o600)
            marker.write_text(json.dumps({'project':self.project,'prefix':self.prefix}))
            marker.chmod(0o600)
        pki = self.work/'pki'
        if not (pki/'webhook.crt').exists(): issue_server(pki,'webhook',self.prefix+'.40.6')
        secret = self.work/'webhook-token'
        if not secret.exists():
            secret.write_text(secrets.token_urlsafe(32));secret.chmod(0o600)
        viewer = self.work/'viewer-token'
        if not viewer.exists():
            viewer.write_text(secrets.token_urlsafe(32));viewer.chmod(0o600)
        password = (self.work/'password').read_text()
        entries = {}
        for name in ['controller','branch','dc']:
            entries[name+'/ca.crt']=(pki/'ca.crt').read_bytes()
            for suffix in ['crt','key']:
                entries[f'{name}/{name}.{suffix}']=(pki/f'{name}.{suffix}').read_bytes()
        entries.update({'controller/'+k:v.encode() for k,v in manifests().items()})
        entries['controller/database.dsn']=f'postgresql://netmonorc:{password}@postgres:5432/netmonorc?sslmode=disable'.encode()
        entries['postgres/password']=password.encode()
        entries['controller/viewer.token']=viewer.read_bytes()
        entries['fixture/mode.json']=json.dumps(NORMAL).encode()
        entries['controller/webhook.yaml']=f'''apiVersion: netmonorc.io/v1alpha1
kind: WebhookEndpoint
metadata:
  name: default-webhook
spec:
  url: https://{self.prefix}.40.6:9444/events
  auth:
    type: bearer
    secretRef: alert-webhook-token
  timeout: 2s
  retry:
    maxAttempts: 5
    initialBackoff: 1s
    maxBackoff: 4s
'''.encode()
        entries['controller/secrets/alert-webhook-token']=secret.read_bytes()
        entries['webhook/token']=secret.read_bytes()
        for suffix in ['crt','key']: entries[f'webhook/webhook.{suffix}']=(pki/f'webhook.{suffix}').read_bytes()
        entries['webhook-data/mode.json']=json.dumps({'generation':secrets.token_hex(8),'fail_next':0}).encode()
        for directory in ['controller-data','branch-data','dc-data','webhook-data']:
            entries[directory+'/.initialized']=b'NetMonOrc private volume\n'
        archive=io.BytesIO()
        with tarfile.open(fileobj=archive,mode='w') as tar:
            for directory in ['controller','controller/secrets','branch','dc','postgres','fixture','controller-data','branch-data','dc-data','webhook','webhook-data']:
                info=tarfile.TarInfo(directory);info.type=tarfile.DIRTYPE;info.mode=0o700
                info.uid=info.gid=0 if directory in ['postgres','fixture'] else 10001
                tar.addfile(info)
            for name,data in entries.items():
                info=tarfile.TarInfo(name);info.size=len(data);info.mode=0o600
                info.uid=info.gid=0 if name.split('/')[0] in ['postgres','fixture'] else 10001
                tar.addfile(info,io.BytesIO(data))
        self.compose('run','--rm','--no-deps','-T','init',input=archive.getvalue())
    def up(self,mode='adaptive',policy='confirmed'):
        self.require()
        profile=('fixed' if mode=='fixed' else 'adaptive')+('-confirmed' if policy=='confirmed' else '')
        started=time.time()
        self.compose('up','-d','--wait','--wait-timeout','90',
                     env={'NMO_PROFILE':profile,'NMO_STATIC':str(mode=='static').lower()})
        self.wait(lambda:self.assessment().get('state')=='healthy' and
                  self.sql("SELECT count(DISTINCT agent_id) FROM nmo_observations WHERE diagnostic_task_id='' AND finished_at>to_timestamp("+str(started)+")")=="2",
                  'fresh healthy observations from both Agents',seconds=45)
    def mode(self):
        raw=self.compose('ps','--all','--quiet','controller').decode().strip()
        if not raw:return None
        values=json.loads(subprocess.check_output(['docker','inspect',raw]))[0]['Config']['Cmd']
        if '-diagnostic-comparison-static=true' in values:return 'static'
        return 'fixed' if any(v.endswith('/fixed.yaml') or v.endswith('/fixed-confirmed.yaml') for v in values) else 'adaptive'
    def snapshot(self,name='assessment.json'):
        # docker cp works with local and remote daemons and needs no host mount.
        try:
            container=self.compose('ps','--all','--quiet','controller').decode().strip()
            if not container:return None
            data=subprocess.run(['docker','cp',container+':/data/'+name,'-'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True).stdout
            with tarfile.open(fileobj=io.BytesIO(data),mode='r:*') as tar:
                return tar.extractfile(tar.getmembers()[0]).read()
        except subprocess.CalledProcessError:
            return None
    def assessment(self):
        data=self.snapshot();return json.loads(data) if data else {}
    def diagnostic(self):
        data=self.snapshot('diagnostic.json');return json.loads(data) if data else None
    def sql(self,query):
        return self.compose('exec','-T','postgres','psql','-U','netmonorc','-d','netmonorc','-tAc',query).decode().strip()
    def inject(self,scenario):
        self.require()
        state=SCENARIOS[scenario]
        code="import os,sys; p='/lab/fixture/mode.json'; f=p+'.pending'; open(f,'wb').write(sys.stdin.buffer.read()); os.replace(f,p)"
        self.compose('run','--rm','--no-deps','-T','--entrypoint','python3','init','-c',code,input=json.dumps(state).encode())
        gateway=self.prefix+'.41.10'
        command=['ip','route','replace',VIP+'/32','via',gateway] if scenario!='office-path' else ['ip','route','del',VIP+'/32']
        # Repeated injection is idempotent, including a route already removed.
        if scenario=='office-path':
            command=['ip','route','replace','prohibit',VIP+'/32']
        self.compose('exec','-T','office-net',*command)
    def wait(self,predicate,description,seconds=45):
        deadline=time.monotonic()+seconds
        while time.monotonic()<deadline:
            value=predicate()
            if value:return value
            time.sleep(.5)
        raise RuntimeError(f'timed out: {description}; inspect docker compose -p {self.project} logs')
    def healthy(self):
        self.inject('healthy')
        return self.wait(lambda:self.assessment().get('state')=='healthy' and
                         self.sql("SELECT count(*) FROM nmo_incidents WHERE episode_id<>''")=='0','healthy/full coverage closes episode')
    def export(self,destination):
        destination=Path(destination);destination.mkdir(parents=True,mode=0o700,exist_ok=True)
        for name in ['assessment.json','diagnostic.json','report.html','alert.json','delivery.json']:
            b=self.snapshot(name)
            if b is not None:
                p=destination/name;p.write_bytes(b);p.chmod(0o600)
    def package(self,destination):
        self.require()
        # Explicit allowlist: never archive configuration volumes, PKI or DSN.
        entries={}
        for name in ['assessment.json','diagnostic.json','report.html','alert.json','delivery.json']:
            data=self.snapshot(name)
            if data is not None:entries[name]=data
        if 'assessment.json' not in entries or 'report.html' not in entries:
            raise RuntimeError('Controller assessment and HTML are required for a demo package')
        entries['README.txt']=b'NetMonOrc private demonstration snapshots. Not a database backup.\nFiles are read separately; inspect their timestamps and revisions.\nNo PKI, token, DSN or spool is included.\nOpen report.html locally. Optional JSON files can rebuild the report CLI.\n'
        entries['SHA256SUMS']=''.join(hashlib.sha256(data).hexdigest()+'  '+name+'\n' for name,data in sorted(entries.items())).encode()
        destination=Path(destination)
        destination.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
        fd=os.open(destination,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        try:
            with os.fdopen(fd,'wb') as output:
                with tarfile.open(fileobj=output,mode='w:gz') as archive:
                    for name,data in sorted(entries.items()):
                        info=tarfile.TarInfo(name);info.size=len(data);info.mode=0o600
                        archive.addfile(info,io.BytesIO(data))
                output.flush();os.fsync(output.fileno())
        except BaseException:
            destination.unlink(missing_ok=True)
            raise

    def backup(self,destination):
        self.require()
        destination=Path(destination)
        destination.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
        # Exclusive private creation avoids exposing the dump before chmod,
        # and never overwrites the user's earlier backup.
        fd=os.open(destination,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        try:
            with os.fdopen(fd,'wb') as output:
                subprocess.run(['docker','compose','-p',self.project,'-f',str(ROOT/'compose.yaml'),
                                'exec','-T','postgres','pg_dump','-U','netmonorc','-d','netmonorc','-Fc'],
                               cwd=ROOT,env=self.env,stdout=output,stderr=subprocess.PIPE,check=True)
                output.flush();os.fsync(output.fileno())
        except BaseException:
            destination.unlink(missing_ok=True)
            raise

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('operation',choices=['prepare','up','status','inject','export','package','backup','down'])
    parser.add_argument('--project',default='netmonorc-lab')
    parser.add_argument('--prefix',default='172.30')
    parser.add_argument('--mode',choices=['static','fixed','adaptive'],default='adaptive')
    parser.add_argument('--policy',choices=['legacy','confirmed'],default='confirmed')
    parser.add_argument('--scenario',choices=list(SCENARIOS),default='healthy')
    parser.add_argument('--output',default='.local/lab-report')
    args=parser.parse_args();lab=Lab(args.project,args.prefix)
    if args.operation=='prepare':lab.prepare();print('Private configuration volumes prepared; CA private key stays on the host.')
    elif args.operation=='up':lab.up(args.mode,args.policy);print('Stand started; no host ports published.')
    else:
        lab.require()
        if args.operation=='status':print(json.dumps({'mode':lab.mode(),'assessment':lab.assessment(),'diagnostic':lab.diagnostic()},indent=2))
        elif args.operation=='inject':lab.inject(args.scenario);print('Injected:',args.scenario)
        elif args.operation=='export':lab.export(args.output);print('Exported private snapshots to',args.output)
        elif args.operation=='package':lab.package(args.output);print('Saved private demo package to',args.output)
        elif args.operation=='backup':lab.backup(args.output);print('Saved private PostgreSQL dump to',args.output)
        elif args.operation=='down':lab.compose('down');print('Stopped; named volumes and data preserved.')
if __name__=='__main__':
    try:main()
    except (RuntimeError,ValueError,FileNotFoundError,FileExistsError) as e:
        raise SystemExit(str(e))
    except subprocess.CalledProcessError as e:
        # Do not dump subprocess arguments or stdin: setup transports secrets.
        raise SystemExit('Docker operation failed: '+e.stderr.decode(errors='replace')[-3000:])
