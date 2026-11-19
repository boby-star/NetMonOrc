"""Owned per-check placement, trusted labels/domains and historical replay acceptance."""
import argparse
import copy
import io
import json
from pathlib import Path
import secrets
import subprocess
import tarfile
from lab import Lab, ROOT
from lab_replicas import CLIENT

class PlacementLab(Lab):
    def compose(self,*args,input=None,env=None):
        override=self.work/'placement.compose.json'
        if not override.exists():return super().compose(*args,input=input,env=env)
        return subprocess.run(['docker','compose','-p',self.project,'-f',str(ROOT/'compose.yaml'),'-f',str(override),*args],cwd=ROOT,env={**self.env,**(env or {})},input=input,stdin=subprocess.DEVNULL if input is None else None,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True).stdout
    def prepare_inventory(self):
        if self.work.exists():raise RuntimeError('Use a new owned project')
        self.prepare()
        command=json.loads(self.compose('config','--format','json'))['services']['controller']['command']
        command.append('-agent-labels-file=/run/netmonorc/agent-labels.json')
        (self.work/'placement.compose.json').write_text(json.dumps({'services':{'controller':{'command':command}}}))
        labels={'branch':{'role':'probe','site':'office'},'dc':{'role':'probe','site':'server'}}
        payload=json.dumps(labels).encode();archive=io.BytesIO()
        with tarfile.open(fileobj=archive,mode='w') as tar:
            item=tarfile.TarInfo('controller/agent-labels.json');item.size=len(payload);item.mode=0o600;item.uid=item.gid=10001;tar.addfile(item,io.BytesIO(payload))
        self.compose('run','--rm','--no-deps','-T','init',input=archive.getvalue())
    def domains(self,value):
        path=self.work/'placement.compose.json';override=json.loads(path.read_text())
        override['services']['controller']['command']=[('-failure-domains='+value) if x.startswith('-failure-domains=') else x for x in override['services']['controller']['command']]
        path.write_text(json.dumps(override))
        self.compose('up','-d','--no-deps','controller')


def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users')!='0':raise RuntimeError('Use a fresh owned project')
    password=secrets.token_urlsafe(32)
    lab.compose('exec','-T','controller','/usr/local/bin/netmonorc-admin','-database-file','/run/netmonorc/database.dsn','-username','placement-admin','-password-stdin',input=password.encode())
    base={'address':lab.prefix+'.40.4','ca':(lab.work/'pki/ca.crt').read_text()}
    def request(method,path,body=None,**extra):
        return json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**base,'method':method,'path':path,'body':body,**extra}).encode()))
    login=request('POST','/api/v1/auth/login',{'username':'placement-admin','password':password});password='';assert login['status']==200
    auth={'cookie':login['cookie'],'csrf':login['body']['csrfToken']};path='/api/v1/resources/Monitor/research-web'
    original=request('GET',path,**auth)['body']['manifest'];cases={}
    def put(manifest):
        current=request('GET',path,**auth);saved=request('PUT',path,manifest,etag=current['etag'],**auth);assert saved['status']==200,saved
        return saved['body']['status']['desiredRevision']
    def wait_state(state):
        def matches():
            current=request('GET',path,**auth)
            if current['status']!=200:return None
            a=lab.assessment()
            return a if a.get('revision')==current['body']['status']['desiredRevision'] and a.get('state')==state else None
        return lab.wait(matches,state+' at exact current revision',seconds=80)
    def sources(revision):
        assert len(revision)==64 and all(c in '0123456789abcdef' for c in revision)
        return json.loads(lab.sql("SELECT COALESCE(json_agg(x),'[]')::text FROM (SELECT DISTINCT agent_id,check_id FROM nmo_observations WHERE revision='"+revision+"' AND diagnostic_task_id='' ORDER BY agent_id,check_id) x"))
    split=copy.deepcopy(original);split['spec']['diagnostics']['enabled']=False
    for check in split['spec']['checks']:
        check['placement']={'strategy':'leastLoaded','selector':{'matchLabels':{'role':'probe'},'matchExpressions':[{'key':'site','operator':'In','values':['office' if check['id']=='http' else 'server']}]},'replicas':1}
    try:
        planned=request('POST','/api/v1/config/plan',{'manifests':[split]},**auth)
        assert planned['status']==200 and len(planned['body']['placements'])==3
        assert sorted(p['checkId'] for p in planned['body']['placements'])==['dns','http','tcp']
        put(split);a=wait_state('healthy');observed=sources(a['revision'])
        assert observed==[{'agent_id':'branch','check_id':'http'},{'agent_id':'dc','check_id':'dns'},{'agent_id':'dc','check_id':'tcp'}],observed
        assert all(z['coverage']=='complete' for z in a['zones'])
        assert lab.sql('SELECT count(*) FROM nmo_check_placements')=='3'
        cases['split']={'state':'healthy','sources':observed,'completeZones':len(a['zones']),'planCheckIds':['dns','http','tcp']}
        # Exact allocations must remain available for old-revision replay.
        bindings=json.loads(lab.sql("SELECT json_agg(json_build_object('agent',agent_id,'checks',allowed_checks) ORDER BY agent_id)::text FROM nmo_assignments WHERE revision='"+a['revision']+"'"))
        assert bindings==[{'agent':'branch','checks':['http']},{'agent':'dc','checks':['dns','tcp']}],bindings
        # Restart preserves per-check selections; unknown capacity is not fabricated.
        revision=a['revision'];lab.compose('restart','controller');a=wait_state('healthy');assert a['revision']==revision
        cases['restart']={'sameRevision':True,'historicalCheckBindings':bindings}
        missing=copy.deepcopy(split);missing['spec']['checks'][0]['placement']['selector']['matchLabels']['role']='absent'
        put(missing);a=wait_state('unknown');observed=sources(a['revision'])
        assert not any(x['check_id']=='http' for x in observed)
        cases['selectorMiss']={'state':'unknown','noUnassignedHTTP':True}
        # Monitor-level failure-domain policy shares the same trusted inventory.
        domains=copy.deepcopy(original);domains['spec']['diagnostics']['enabled']=False
        domains['spec']['placement']={'strategy':'leastLoaded','selector':{'matchLabels':{'role':'probe'}},'replicas':2,'distinctBy':'failureDomain'}
        put(domains);a=wait_state('healthy');assert len(sources(a['revision']))==6
        cases['distinctDomains']={'state':'healthy','physicalPairs':6}
        lab.domains('branch=shared,dc=shared');a=wait_state('unknown');assert sources(a['revision'])==[]
        cases['sharedDomains']={'state':'unknown','physicalPairs':0}
        lab.domains('branch=known');a=wait_state('unknown');assert sources(a['revision'])==[]
        cases['unknownDomain']={'state':'unknown','physicalPairs':0}
    finally:
        lab.domains('branch=office-path,dc=server-path')
        lab.wait(lambda:request('GET','/api/v1/auth/me',**auth)['status']==200,'HTTP after metadata restoration')
        put(original);wait_state('healthy')
    agents=request('GET','/api/v1/agents',**auth);assert agents['status']==200
    assert all(a['labels']['role']=='probe' for a in agents['body']['items'])
    assert request('POST','/api/v1/auth/logout',**auth)['status']==204
    return {'status':'passed','trustedHTTPS':True,'realPostgreSQLAndAgentProbes':True,'cases':cases,'restoredHealthy':True,'browserInteractionTested':False,'credentialsOmitted':True}

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('operation',choices=['prepare','up','verify','down']);parser.add_argument('--project',required=True);parser.add_argument('--prefix',required=True);parser.add_argument('--output')
    args=parser.parse_args();lab=PlacementLab(args.project,args.prefix)
    if args.operation=='prepare':lab.prepare_inventory();print('Private placement inventory prepared')
    elif args.operation=='up':lab.up();print('Placement fixture running')
    elif args.operation=='down':lab.compose('down');print('Stopped; data and volumes preserved')
    else:
        path=Path(args.output)
        if path.exists():raise FileExistsError('Use a new output path')
        result=verify(lab);path.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
        with path.open('x') as f:json.dump(result,f,indent=2);f.write('\n')
        path.chmod(0o600);print('PASS: per-check allocations, metadata selectors, distinct/shared/unknown domains and restart persistence; secrets omitted')
