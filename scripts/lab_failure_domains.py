"""Real confirmation with independent, shared and unknown Controller-owned domains."""
import argparse
import json
from pathlib import Path
import secrets
import subprocess
from lab import Lab, ROOT
from lab_replicas import CLIENT

class DomainLab(Lab):
    def compose(self,*args,input=None,env=None):
        override=self.work/'domains.compose.json'
        if not override.exists():return super().compose(*args,input=input,env=env)
        return subprocess.run(['docker','compose','-p',self.project,'-f',str(ROOT/'compose.yaml'),'-f',str(override),*args],cwd=ROOT,env={**self.env,**(env or {})},input=input,stdin=subprocess.DEVNULL if input is None else None,stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True).stdout
    def domains(self,mapping):
        config=json.loads(Lab.compose(self,'config','--format','json'))
        command=[arg for arg in config['services']['controller']['command'] if not arg.startswith('-failure-domains=')]
        command.append('-failure-domains='+','.join(agent+'='+domain for agent,domain in mapping.items()))
        path=self.work/'domains.compose.json'
        path.write_text(json.dumps({'services':{'controller':{'command':command}}}));path.chmod(0o600)

def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users')!='0':raise RuntimeError('Use a fresh owned project; existing users are preserved')
    password=secrets.token_urlsafe(32)
    lab.compose('exec','-T','controller','/usr/local/bin/netmonorc-admin','-database-file','/run/netmonorc/database.dsn','-username','domain-admin','-password-stdin',input=password.encode())
    base={'address':lab.prefix+'.40.4','ca':(lab.work/'pki/ca.crt').read_text()}
    def request(method,path,body=None,**extra):
        return json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**base,'method':method,'path':path,'body':body,**extra}).encode()))
    login=request('POST','/api/v1/auth/login',{'username':'domain-admin','password':password});password=''
    assert login['status']==200
    auth={'cookie':login['cookie'],'csrf':login['body']['csrfToken']}
    cases={}
    for name,mapping,count in [('distinct',{'branch':'office-path','dc':'server-path'},2),('shared',{'branch':'shared-gateway','dc':'shared-gateway'},1),('unknown',{},0)]:
        lab.healthy();lab.domains(mapping);lab.up()
        agents=request('GET','/api/v1/agents',**auth);assert agents['status']==200
        observed={a['agentId']:a.get('failureDomain','') for a in agents['body']['items']}
        assert observed=={agent:mapping.get(agent,'') for agent in ['branch','dc']},observed
        prior=lab.diagnostic();prior_id=prior['id'] if prior else None
        try:
            lab.inject('http')
            def terminal():
                r=lab.diagnostic()
                return r if r and r['id']!=prior_id and r.get('finishedAt') else None
            report=lab.wait(terminal,'terminal '+name+'-domain diagnostic',seconds=90)
            result=report['result'];confirmation=result['confirmation']
            assert confirmation['distinctFailureDomains']==count,confirmation
            assert confirmation['satisfied']==(count==2),confirmation
            assert result['status']==('completed' if count==2 else 'inconclusive'),result
            evidence_domains=confirmation['evidenceDomains'];assert evidence_domains
            assert {d for d in evidence_domains.values() if d}==set(mapping.values())
            detail=request('GET','/api/v1/diagnostics/'+report['id'],**auth);assert detail['status']==200
            assert detail['body']['explanation']['confirmation']['evidenceDomains']==evidence_domains
            for decision in detail['body']['explanation']['decisions']:
                assert decision.get('failureDomain','')==mapping.get(decision['agentId'],'')
            cases[name]={'status':result['status'],'satisfied':confirmation['satisfied'],'distinctDomains':count,'observations':confirmation['observations'],'evidenceDomains':evidence_domains,'agentDomains':observed,'checks':report['reservedChecks']}
        finally:lab.healthy()
    assert request('POST','/api/v1/auth/logout',**auth)['status']==204
    return {'status':'passed','twoRealAgents':True,'trustedHTTPS':True,'domainsFromControllerOnly':True,'historicalDomainsPersistedInAPI':True,'browserInteractionTested':False,'cases':cases,'credentialsOmitted':True}

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',required=True);p.add_argument('--output',required=True);a=p.parse_args()
    path=Path(a.output)
    if path.exists():raise FileExistsError('Use a new output path')
    result=verify(DomainLab(a.project,a.prefix))
    path.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    with path.open('x') as f:json.dump(result,f,indent=2);f.write('\n')
    path.chmod(0o600)
    print('PASS: distinct/shared/unknown domains and persisted confirmation; secrets omitted')
