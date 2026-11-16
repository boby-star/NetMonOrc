"""Live dynamic regular placement, restart and loss-of-Agent workflow."""
import copy
import json
import subprocess
import uuid


def verify(lab, cli, common, first, client):
    def command(*args, value=None):
        return json.loads(lab.compose('exec','-T','controller',*cli,args[0],'--context-file','/tmp/cli/context.json',*args[1:],input=json.dumps(value).encode() if value is not None else None))
    def apply(manifests, wait=True):
        return command('apply','-f','-',*(['--wait','--timeout','90s'] if wait else []),value={'manifests':manifests,'expectedVersions':{},'requestId':str(uuid.uuid4())})
    def selection():
        return json.loads(lab.sql("SELECT coalesce(jsonb_object_agg(r.name,p.selected),'{}') FROM nmo_regular_placements p JOIN nmo_resources r ON r.uid=p.monitor_uid"))
    def states():
        return json.loads(lab.sql("SELECT coalesce(jsonb_object_agg(r.name,a.payload->>'state'),'{}') FROM nmo_assessments a JOIN nmo_resources r ON r.name=a.monitor AND r.kind='Monitor' AND r.effective_revision=a.revision"))
    def sources(expected):
        values=selection()
        return all([s['Agent'] for s in values.get(name,[])]==agents for name,agents in expected.items())
    def healthy():
        values=states()
        return values.get('dynamic-a')=='healthy' and values.get('dynamic-b')=='healthy'
    original=command('get','Monitor','research-web')
    policy={'apiVersion':'netmonorc.io/v1alpha1','kind':'PlacementPolicy','metadata':{'name':'dynamic-pool'},'spec':{'strategy':'roundRobin','replicas':1,'distinctBy':'zone'}}
    monitors=[]
    for name in ['dynamic-a','dynamic-b']:
        m=copy.deepcopy(original['manifest']);m['metadata']={'name':name}
        m['spec']['checks']=[c for c in m['spec']['checks'] if c['id']=='http']
        m['spec']['placementRef']='dynamic-pool';m['spec']['diagnostics']={'enabled':False}
        m['spec']['assessment']={'checks':[{'id':'http','weight':1,'required':True}],'freshness':{'maxAge':'20s','maxSkew':'15s'}}
        monitors.append(m)
    operation=apply([policy,*monitors]);assert operation['state']=='completed'
    lab.wait(lambda:healthy() and sources({'dynamic-a':['branch'],'dynamic-b':['dc']}),'two roundRobin assignments reach real healthy probes',seconds=60)
    before={name:command('get','Monitor',name) for name in ['dynamic-a','dynamic-b']}
    assert all(command('get','Monitor',name)['resourceVersion']==v['resourceVersion'] for name,v in before.items())
    at=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>at and healthy() and sources({'dynamic-a':['branch'],'dynamic-b':['dc']}),'assignment and cursor survive Controller restart',seconds=60)
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    stopped=False
    try:
        lab.compose('stop','branch');stopped=True
        lab.wait(lambda:healthy() and sources({'dynamic-a':['dc'],'dynamic-b':['dc']}),'lost Agent is replaced without moving the other Monitor',seconds=90)
        after={name:command('get','Monitor',name) for name in ['dynamic-a','dynamic-b']}
        assert after['dynamic-a']['status']['desiredRevision']!=before['dynamic-a']['status']['desiredRevision']
        assert after['dynamic-b']['status']['desiredRevision']==before['dynamic-b']['status']['desiredRevision']
        assert all(after[name]['resourceVersion']==v['resourceVersion'] for name,v in before.items())
        assert lab.assessment()['state']!='down'
        pending=copy.deepcopy(monitors[0]);pending['metadata']={'name':'dynamic-pending'}
        pending['spec'].pop('placementRef');pending['spec']['placement']={'strategy':'leastLoaded','agentRefs':['dc'],'replicas':2,'onInsufficientAgents':'pending'}
        admission=apply([pending],wait=False)
        lab.wait(lambda:'dynamic-pending' in selection(),'pending selection persisted',seconds=45)
        assert selection()['dynamic-pending']==[]
        assert lab.sql("SELECT count(*) FROM nmo_observations WHERE monitor='dynamic-pending'")=='0'
        assert command('get','operations',admission['id'])['state']!='completed'
        loaded=copy.deepcopy(pending);loaded['metadata']={'name':'dynamic-loaded'}
        loaded['spec']['placement']={'agentRefs':['dc'],'replicas':1}
        assert apply([loaded])['state']=='completed'
        lab.wait(lambda:states().get('dynamic-loaded')=='healthy' and sources({'dynamic-loaded':['dc']}),'default leastLoaded yields an authorized real probe',seconds=60)

    finally:
        if stopped:lab.compose('start','branch')
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and healthy(),'regular baseline and both Monitor recover',seconds=60)
    assert sources({'dynamic-a':['dc'],'dynamic-b':['dc']})
    for name in ['dynamic-pending','dynamic-loaded','dynamic-a','dynamic-b']:
        deleted=json.loads(lab.compose('exec','-T','office-net','python3','-c',client,input=json.dumps({**common,**first,'mode':'rule-delete','kind':'Monitor','name':name}).encode()))
        assert deleted['deleted']
        lab.wait(lambda:name not in selection(),'deleted assignment state cascades',seconds=30)
    deleted=json.loads(lab.compose('exec','-T','office-net','python3','-c',client,input=json.dumps({**common,**first,'mode':'rule-delete','kind':'PlacementPolicy','name':'dynamic-pool'}).encode()))
    assert deleted['deleted']
    def restored():
        try:agents=command('get','agents')['items']
        except subprocess.CalledProcessError:return False
        return lab.assessment().get('state')=='healthy' and len(agents)==2 and all(a['onlineState']=='online' for a in agents)
    lab.wait(restored,'original healthy and current online Agent proof remain after cleanup')
    return {'roundRobinPhysicalAssignments':True,'defaultLeastLoadedPhysicalProbe':True,'applyCompletesAfterDerivedPlacementACK':True,'assignmentPersistsAcrossRestart':True,'lostAgentReassigned':True,'unrelatedMonitorRevisionPreserved':True,'sourceResourceVersionsPreserved':True,'insufficientCandidatesStayPending':True,'noPartialPhysicalProbes':True,'agentLossNotServiceDown':True,'stickyRecoveryAvoidsChurn':True,'createdResourcesRemoved':True}
