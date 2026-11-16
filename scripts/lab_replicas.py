"""Owned two-Agent same-zone acceptance with real opposing probe outcomes."""
import argparse
import io
import json
from pathlib import Path
import secrets
import subprocess
import tarfile
from lab import Lab, ROOT

CLIENT=r'''
import http.client,json,socket,ssl,sys,time
from http.cookies import SimpleCookie
c=json.load(sys.stdin);ctx=ssl.create_default_context(cadata=c['ca'])
def request():
    body=json.dumps(c['body']).encode() if c.get('body') is not None else b''
    headers={'Host':'localhost','Content-Length':str(len(body)),'Connection':'close'}
    if body:headers['Content-Type']='application/json'
    if c.get('cookie'):headers['Cookie']=c['cookie']
    if c.get('csrf'):headers['X-CSRF-Token']=c['csrf']
    if c.get('etag'):headers['If-Match']=c['etag']
    with socket.create_connection((c['address'],8443),timeout=8) as raw:
        with ctx.wrap_socket(raw,server_hostname='localhost') as sock:
            sock.sendall((c['method']+' '+c['path']+' HTTP/1.1\r\n'+''.join(k+': '+v+'\r\n' for k,v in headers.items())+'\r\n').encode()+body)
            r=http.client.HTTPResponse(sock);r.begin();data=r.read(32768)
            result={'status':r.status,'body':json.loads(data) if data else None,'etag':r.getheader('ETag')}
            if c['path']=='/api/v1/auth/login' and r.status==200:
                cookie=SimpleCookie();cookie.load(r.getheader('Set-Cookie'));result['cookie']='netmonorc_session='+cookie['netmonorc_session'].value
            return result
for attempt in range(20):
    try:
        result=request()
        if c['method']!='GET' or result['status'] not in [429,503]:break
    except (OSError,http.client.HTTPException):
        if c['method']!='GET' or attempt==19:raise
    time.sleep(.5)
print(json.dumps(result))
'''

class ReplicaLab(Lab):
    def compose(self,*args,input=None,env=None):
        override=self.work/'replicas.compose.json'
        if not override.exists():return super().compose(*args,input=input,env=env)
        return subprocess.run(['docker','compose','-p',self.project,'-f',str(ROOT/'compose.yaml'),'-f',str(override),*args],cwd=ROOT,env={**self.env,**(env or {})},input=input,stdin=subprocess.DEVNULL if input is None else None,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True).stdout
    def prepare_replicas(self):
        if self.work.exists():raise RuntimeError('Use a new owned project; existing configuration is preserved')
        self.prepare()
        command=json.loads(self.compose('config','--format','json'))['services']['controller']['command']
        command=['-slots=branch=office,dc=office' if v.startswith('-slots=') else v for v in command]
        override={'services':{'controller':{'command':command}}}
        (self.work/'replicas.compose.json').write_text(json.dumps(override));(self.work/'replicas.compose.json').chmod(0o600)
        manifest={'apiVersion':'netmonorc.io/v1alpha1','kind':'Monitor','metadata':{'name':'research-web'},'spec':{'target':{'address':'198.18.0.80'},'checks':[{'id':'http','type':'http','runner':'blackbox','interval':'10s','timeout':'2s','config':{'url':'http://198.18.0.80:8080/','expectedStatus':[200]}}],'assessment':{'requiredZones':['office'],'freshness':{'maxAge':'20s','maxSkew':'15s'},'checks':[{'id':'http','weight':1,'required':True}]},'diagnostics':{'enabled':False}}}
        payload=json.dumps(manifest).encode();archive=io.BytesIO()
        with tarfile.open(fileobj=archive,mode='w') as tar:
            item=tarfile.TarInfo('controller/monitor.yaml');item.size=len(payload);item.mode=0o600;item.uid=item.gid=10001;tar.addfile(item,io.BytesIO(payload))
        self.compose('run','--rm','--no-deps','-T','init',input=archive.getvalue())

def verify(lab):
    password=secrets.token_urlsafe(32)
    lab.compose('exec','-T','controller','/usr/local/bin/netmonorc-admin','-database-file','/run/netmonorc/database.dsn','-username','replica-admin','-password-stdin',input=password.encode())
    base={'address':lab.prefix+'.40.4','ca':(lab.work/'pki/ca.crt').read_text()}
    def request(method,path,body=None,**extra):
        return json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**base,'method':method,'path':path,'body':body,**extra}).encode()))
    login=request('POST','/api/v1/auth/login',{'username':'replica-admin','password':password});password=''
    assert login['status']==200
    auth={'cookie':login['cookie'],'csrf':login['body']['csrfToken']};path='/api/v1/resources/Monitor/research-web'
    original=request('GET',path,**auth)['body']['manifest']
    def policy(mode):
        current=request('GET',path,**auth);assert current['status']==200
        manifest=current['body']['manifest'];manifest['spec']['assessment']['sameZoneAggregation']=mode
        applied=request('PUT',path,manifest,etag=current['etag'],**auth);assert applied['status']==200
        return applied['body']['status']['desiredRevision']
    def wait_case(name,state,coverage,revision=None):
        def matches():
            a=lab.assessment();zones=a.get('zones',[])
            return a.get('state')==state and len(zones)==1 and zones[0]['coverage']==coverage and (revision is None or a.get('revision')==revision)
        lab.wait(matches,name,seconds=60)
        a=lab.assessment();z=a['zones'][0]
        return {'state':a['state'],'coverage':z['coverage'],'q':z['q'],'h':z.get('h'),'reason':z['reason'],'evidenceCount':len(z['evidence'])}
    cases={'bothHealthy':wait_case('both same-zone physical sources healthy','healthy','complete')}
    try:
        lab.inject('office-path')
        cases['allMixed']=wait_case('one negative and one positive under all','down','complete')
        assert cases['allMixed']['evidenceCount']==2 and cases['allMixed']['reason']=='AGENT_RESULTS_DIVERGE'
        revision=policy('any');cases['anyMixed']=wait_case('any retains positive physical path','healthy','complete',revision)
        revision=policy('quorum');cases['quorumTie']=wait_case('strict majority of two is not satisfied','unknown','complete',revision)
        lab.compose('stop','dc')
        cases['quorumMissing']=wait_case('missing vote does not shrink planned quorum','unknown','coverage_degraded',revision)
        revision=policy('all');cases['requiredFailureWithoutPeer']=wait_case('one required all failure independently proves down','down','coverage_degraded',revision)
    finally:
        lab.inject('healthy');lab.compose('start','dc')
    current=request('GET',path,**auth);assert current['status']==200
    assert request('PUT',path,original,etag=current['etag'],**auth)['status']==200
    cases['recovered']=wait_case('both sources recover after restoring policy','healthy','complete')
    assert request('POST','/api/v1/auth/logout',**auth)['status']==204
    assert request('GET','/api/v1/auth/me',cookie=auth['cookie'])['status']==401
    return {'status':'passed','twoPhysicalAgentsSameZone':True,'trustedHTTPS':True,'browserInteractionTested':False,'opposingOutcomesFromRealNetworkFault':True,'strictPlannedQuorum':True,'requiredFailureInvariant':True,'cases':cases,'credentialsOmitted':True}

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',default='172.19');p.add_argument('--output',required=True);args=p.parse_args()
    output=Path(args.output)
    if output.exists():raise FileExistsError('Use a new output path')
    lab=ReplicaLab(args.project,args.prefix);lab.prepare_replicas()
    try:
        lab.up();result=verify(lab)
        output.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
        with output.open('x') as target:json.dump(result,target,indent=2);target.write('\n')
        output.chmod(0o600);print('PASS: real same-zone replicas all/any/quorum; credentials omitted')
    finally:lab.compose('down')
