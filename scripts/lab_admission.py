"""Two real Monitor diagnoses share one Agent; regular observations continue."""
import argparse
import copy
import json
from pathlib import Path
import secrets
from lab import Lab
from lab_replicas import CLIENT

def verify(lab):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users')!='0':raise RuntimeError('Use a fresh owned project; existing users are preserved')
    password=secrets.token_urlsafe(32)
    lab.compose('exec','-T','controller','/usr/local/bin/netmonorc-admin','-database-file','/run/netmonorc/database.dsn','-username','admission-admin','-password-stdin',input=password.encode())
    base={'address':lab.prefix+'.40.4','ca':(lab.work/'pki/ca.crt').read_text()}
    def request(method,path,body=None,**extra):
        return json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**base,'method':method,'path':path,'body':body,**extra}).encode()))
    login=request('POST','/api/v1/auth/login',{'username':'admission-admin','password':password});password=''
    assert login['status']==200
    auth={'cookie':login['cookie'],'csrf':login['body']['csrfToken']}
    profile={'apiVersion':'netmonorc.io/v1alpha1','kind':'DiagnosticProfile','metadata':{'name':'admission-slow'},'spec':{
        'mode':'fixed','budget':{'maxAdditionalChecks':1,'maxDuration':'60s','maxParallel':1},
        'actions':[{'id':'slow-http','check':{'type':'http','runner':'native-http','timeout':'15s','config':{'url':'http://198.18.0.80:8080/slow','expectedStatus':[200]}},'placement':{'agentRefs':['dc'],'zones':['server'],'strategy':'fixed'}}],
        'fixed':{'steps':[{'id':'slow-step','actionRef':'slow-http'}]}}}
    assert request('POST','/api/v1/resources/DiagnosticProfile',profile,**auth)['status']==201
    current=request('GET','/api/v1/resources/Monitor/research-web',**auth);assert current['status']==200
    peer=copy.deepcopy(current['body']['manifest']);peer['metadata']={'name':'admission-peer'}
    assert request('POST','/api/v1/resources/Monitor',peer,**auth)['status']==201
    def ready():
        a=request('GET','/api/v1/monitors/admission-peer/assessment',**auth)
        return a['status']==200 and a['body']['state']=='healthy'
    lab.wait(ready,'healthy second Monitor after normal rollout',seconds=60)
    runs=[]
    for monitor in ['research-web','admission-peer']:
        response=request('POST','/api/v1/monitors/'+monitor+'/diagnostics',{'profileRef':'admission-slow','requestId':secrets.token_hex(16)},**auth)
        assert response['status'] in [200,202],response['status']
        run=response['body']['id'];assert len(run)==32 and all(c in '0123456789abcdef' for c in run)
        runs.append(run)
    quoted=','.join("'"+r+"'" for r in runs)
    lab.wait(lambda:lab.sql('SELECT count(*) FROM nmo_diagnostic_tasks WHERE run_id IN ('+quoted+") AND state='accepted'")=='1','one accepted physical task')
    waiting=json.loads(lab.sql('SELECT json_agg(json_build_object(\'id\',id,\'used\',used_checks,\'state\',state)) FROM nmo_diagnostic_runs WHERE id IN ('+quoted+')'))
    assert sorted(row['used'] for row in waiting)==[0,1] and all(row['state']=='running' for row in waiting),waiting
    lab.wait(lambda:lab.sql('SELECT count(*) FROM nmo_diagnostic_runs WHERE id IN ('+quoted+") AND state<>'running'")=='2','both queued diagnoses finish',seconds=60)
    observations=json.loads(lab.sql("SELECT json_agg(o.payload ORDER BY o.finished_at) FROM nmo_observations o JOIN nmo_diagnostic_tasks t ON t.id=o.diagnostic_task_id WHERE t.run_id IN ("+quoted+')'))
    assert len(observations)==2 and all(o['execution']=='completed' and o['outcome']=='success' and o['agent']=='dc' for o in observations),observations
    assert observations[1]['startedAt']>=observations[0]['finishedAt'],'physical diagnostic probes overlap'
    regular=int(lab.sql("SELECT count(*) FROM nmo_observations WHERE diagnostic_task_id='' AND agent_id='dc' AND finished_at>='"+observations[0]['startedAt']+"'::timestamptz AND finished_at<='"+observations[0]['finishedAt']+"'::timestamptz"))
    assert regular>0,'regular monitoring was starved'
    assert lab.sql('SELECT sum(used_checks) FROM nmo_diagnostic_runs WHERE id IN ('+quoted+')')=='2'
    assert request('POST','/api/v1/auth/logout',**auth)['status']==204
    return {'status':'passed','sharedAgent':'dc','twoMonitors':True,'waitingUsedChecks':[0,1],'physicalDiagnosticProbes':2,'physicalIntervalsDoNotOverlap':True,'regularObservationsDuringFirstDiagnostic':regular,'usedChecksTotal':2,'observations':observations,'credentialsOmitted':True}

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',required=True);p.add_argument('--output',required=True);a=p.parse_args()
    path=Path(a.output)
    if path.exists():raise FileExistsError('Use a new output path')
    result=verify(Lab(a.project,a.prefix))
    path.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    with path.open('x') as f:json.dump(result,f,indent=2);f.write('\n')
    path.chmod(0o600)
    print('PASS: two diagnoses share one Agent without overlapping or starving regular probes; secrets omitted')
