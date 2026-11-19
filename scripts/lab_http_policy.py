"""Owned HTTP pin/Host/header/redirect policy acceptance using real Agent probes."""
import argparse
import copy
import json
from pathlib import Path
import secrets
from lab import Lab
from lab_replicas import CLIENT


def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users')!='0':raise RuntimeError('Use a fresh owned project')
    password=secrets.token_urlsafe(32)
    lab.compose('exec','-T','controller','/usr/local/bin/netmonorc-admin','-database-file','/run/netmonorc/database.dsn','-username','http-policy-admin','-password-stdin',input=password.encode())
    base={'address':lab.prefix+'.40.4','ca':(lab.work/'pki/ca.crt').read_text()}
    def request(method,path,body=None,**extra):
        return json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**base,'method':method,'path':path,'body':body,**extra}).encode()))
    login=request('POST','/api/v1/auth/login',{'username':'http-policy-admin','password':password});password='';assert login['status']==200
    auth={'cookie':login['cookie'],'csrf':login['body']['csrfToken']};path='/api/v1/resources/Monitor/research-web'
    original=request('GET',path,**auth)['body']['manifest'];cases={}
    def save(manifest):
        current=request('GET',path,**auth);response=request('PUT',path,manifest,etag=current['etag'],**auth);assert response['status']==200
        return response['body']['status']['desiredRevision']
    def state(revision,want):
        return lab.wait(lambda:lab.assessment().get('revision')==revision and lab.assessment().get('state')==want,want+' at exact HTTP policy revision',seconds=60)
    def results(revision):
        assert len(revision)==64 and all(c in '0123456789abcdef' for c in revision)
        return json.loads(lab.sql("SELECT COALESCE(json_agg(x),'[]')::text FROM (SELECT DISTINCT agent_id,payload->>'execution' AS execution,payload->>'outcome' AS outcome,payload->>'reason' AS reason FROM nmo_observations WHERE revision='"+revision+"' AND check_id='http' AND diagnostic_task_id='' ORDER BY agent_id) x"))
    modified=copy.deepcopy(original);modified['spec']['diagnostics']['enabled']=False
    http=next(c for c in modified['spec']['checks'] if c['id']=='http');http['runner']='auto'
    http['config'].update({'url':'http://service.invalid:8080/policy-redirect','connectIP':'198.18.0.80','headers':{'X-Probe':'safe'},'followRedirects':True})
    try:
        revision=save(modified);state(revision,'healthy');good=results(revision)
        assert len(good)==2 and all(x['execution']=='completed' and x['outcome']=='success' for x in good),good
        cases['pinnedRedirect']={'state':'healthy','hostPreserved':True,'headerVerifiedAtFinalHop':True,'observations':good}
        http['config']['url']='http://service.invalid:8080/policy-denied'
        revision=save(modified);state(revision,'unknown')
        lab.wait(lambda:len(results(revision))==2,'both policy-denied HTTP outcomes')
        denied=results(revision)
        assert all(x['execution']=='execution_failed' and x['outcome']=='unknown' and x['reason']=='target_policy_denied' for x in denied),denied
        cases['deniedRedirect']={'state':'unknown','observations':denied}
    finally:
        restored=save(original);state(restored,'healthy')
    lab.wait(lambda:request('GET','/api/v1/auth/me',**auth)['status']==200,'restored HTTP API')
    assert request('POST','/api/v1/auth/logout',**auth)['status']==204
    return {'status':'passed','trustedHTTPSAPI':True,'realPostgreSQLAndAgentProbes':True,'cases':cases,'restoredHealthy':True,'browserInteractionTested':False,'credentialsOmitted':True}

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--project',required=True);parser.add_argument('--prefix',required=True);parser.add_argument('--output',required=True);args=parser.parse_args()
    path=Path(args.output)
    if path.exists():raise FileExistsError('Use a new output path')
    result=verify(Lab(args.project,args.prefix));path.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    with path.open('x') as f:json.dump(result,f,indent=2);f.write('\n')
    path.chmod(0o600);print('PASS: real Agent connectIP/Host/headers/redirect and denied-target classification; secrets omitted')
