"""Two real physical actions must obey Monitor TTL/maxSkew in diagnostic conclusions."""
import argparse
import json
from pathlib import Path
import secrets
from lab import Lab
from lab_replicas import CLIENT

def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users')!='0':raise RuntimeError('Use a fresh owned project; existing users are preserved')
    password=secrets.token_urlsafe(32)
    lab.compose('exec','-T','controller','/usr/local/bin/netmonorc-admin','-database-file','/run/netmonorc/database.dsn','-username','temporal-admin','-password-stdin',input=password.encode())
    base={'address':lab.prefix+'.40.4','ca':(lab.work/'pki/ca.crt').read_text()}
    def request(method,path,body=None,**extra):
        return json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**base,'method':method,'path':path,'body':body,**extra}).encode()))
    login=request('POST','/api/v1/auth/login',{'username':'temporal-admin','password':password});password=''
    assert login['status']==200
    auth={'cookie':login['cookie'],'csrf':login['body']['csrfToken']}
    profile={'apiVersion':'netmonorc.io/v1alpha1','kind':'DiagnosticProfile','metadata':{'name':'temporal-evidence'},'spec':{
        'mode':'fixed','budget':{'maxAdditionalChecks':2,'maxDuration':'60s','maxParallel':1},
        'hypotheses':[{'id':'class-a'},{'id':'class-b'}],
        'actions':[
            {'id':'early','check':{'type':'http','runner':'native-http','timeout':'2s','config':{'url':'http://198.18.0.80:8080/','expectedStatus':[200]}},'expectedOutcomes':{'class-a':'success','class-b':'failure'},'placement':{'agentRefs':['dc'],'zones':['server'],'strategy':'fixed'}},
            {'id':'late','check':{'type':'http','runner':'native-http','timeout':'15s','config':{'url':'http://198.18.0.80:8080/slow','expectedStatus':[200]}},'expectedOutcomes':{'class-a':'either','class-b':'success'},'placement':{'agentRefs':['dc'],'zones':['server'],'strategy':'fixed'}}],
        'fixed':{'steps':[{'id':'early','actionRef':'early'},{'id':'late','actionRef':'late','dependsOn':['early']}]}}}
    assert request('POST','/api/v1/resources/DiagnosticProfile',profile,**auth)['status']==201
    path='/api/v1/resources/Monitor/research-web';cases={}
    original=request('GET',path,**auth)['body']['manifest']
    try:
        for name,age,skew,expected in [('coherent','20s','15s','completed'),('maxSkew','20s','5s','inconclusive'),('TTL','10s','15s','inconclusive')]:
            current=request('GET',path,**auth);manifest=current['body']['manifest']
            manifest['spec']['assessment']['freshness']={'maxAge':age,'maxSkew':skew}
            saved=request('PUT',path,manifest,etag=current['etag'],**auth);assert saved['status']==200
            revision=saved['body']['status']['desiredRevision']
            lab.wait(lambda:lab.assessment().get('revision')==revision and lab.assessment().get('state')=='healthy','fresh healthy revised Monitor',seconds=60)
            lab.wait(lambda:lab.sql("SELECT count(*) FROM nmo_incidents WHERE episode_id<>''")=='0','previous healthy episode closed')
            started=request('POST','/api/v1/monitors/research-web/diagnostics',{'profileRef':'temporal-evidence','requestId':secrets.token_hex(16)},**auth)
            assert started['status'] in [200,202],started['status']
            run=started['body']['id'];assert len(run)==32 and all(c in '0123456789abcdef' for c in run)
            def terminal():
                r=request('GET','/api/v1/diagnostics/'+run,**auth)
                return r['body'] if r['status']==200 and r['body']['state'] not in ['running','created'] else None
            detail=lab.wait(terminal,'terminal '+name+' diagnostic',seconds=65)
            assert detail['state']==expected,detail
            assert detail['budgetUsed']['additionalChecks']==2
            explanation=detail['explanation'];decisions=explanation['decisions']
            assert len(decisions)==2 and all(d['executionStatus']=='completed' and d['probeOutcome']=='success' for d in decisions)
            for d in decisions:
                proof=request('GET','/api/v1/observations/'+d['observationId'],**auth)
                assert proof['status']==200 and proof['body']['diagnosticSessionId']==run
                assert proof['body']['finishedAt']==d['finishedAt'],'delivery time substituted for physical finish'
                assert d['validUntil']>d['finishedAt']
            assert detail['remainingHypotheses']==(['class-a'] if name=='coherent' else ['class-a','class-b'])
            cases[name]={'state':detail['state'],'hypotheses':detail['remainingHypotheses'],'usedChecks':2,'evidencePolicy':explanation['evidencePolicy'],'decisions':decisions}
    finally:
        current=request('GET',path,**auth)
        restored=request('PUT',path,original,etag=current['etag'],**auth);assert restored['status']==200
        restored_revision=restored['body']['status']['desiredRevision']
    lab.wait(lambda:lab.assessment().get('revision')==restored_revision and lab.assessment().get('state')=='healthy','healthy restored Monitor revision',seconds=60)
    lab.wait(lambda:request('GET','/api/v1/auth/me',**auth)['status']==200,'restored HTTP runtime')
    assert request('POST','/api/v1/auth/logout',**auth)['status']==204
    return {'status':'passed','trustedHTTPS':True,'realPostgreSQLAndAgentProbes':True,'physicalCompletionMatchesObservation':True,'budgetNeverRefunded':True,'browserInteractionTested':False,'cases':cases,'credentialsOmitted':True}

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',required=True);p.add_argument('--output',required=True);a=p.parse_args()
    path=Path(a.output)
    if path.exists():raise FileExistsError('Use a new output path')
    result=verify(Lab(a.project,a.prefix))
    path.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    with path.open('x') as f:json.dump(result,f,indent=2);f.write('\n')
    path.chmod(0o600)
    print('PASS: physical diagnostic timestamps, TTL/maxSkew and unchanged spent budget; secrets omitted')
