"""Verify browser sessions on a fresh, disposable owned Compose project.

The generated test password exists only in memory and stdin, never in argv,
files or output. Use interactive netmonorc-admin for a human installation.
"""
import argparse
import json
from pathlib import Path
import secrets
from lab import Lab

CLIENT=r'''
import http.client,json,socket,ssl,sys
from http.cookies import SimpleCookie
c=json.load(sys.stdin);ctx=ssl.create_default_context(cadata=c['ca'])
cookie=''
def request(method,path,body=None,csrf='',origin='https://localhost',bearer='',if_match='',response_header='Set-Cookie'):
    global cookie
    data=json.dumps(body).encode() if body is not None else b''
    headers={'Host':'localhost','Connection':'close','Content-Length':str(len(data))}
    if body is not None:headers['Content-Type']='application/json'
    if cookie:headers['Cookie']=cookie
    if csrf:headers['X-CSRF-Token']=csrf
    if bearer:headers['Authorization']='Bearer '+bearer
    if if_match:headers['If-Match']=if_match
    if origin:headers['Origin']=origin
    with socket.create_connection((c['address'],8443),timeout=8) as raw:
        with ctx.wrap_socket(raw,server_hostname='localhost') as sock:
            sock.sendall((method+' '+path+' HTTP/1.1\r\n'+''.join(k+': '+v+'\r\n' for k,v in headers.items())+'\r\n').encode()+data)
            response=http.client.HTTPResponse(sock);response.begin();payload=response.read(16384)
            return response.status,payload,response.getheader(response_header)
if c['mode']=='login':
    code,_,_=request('POST','/api/v1/auth/login',{'username':'admin','password':c['password']},origin='https://foreign.example')
    assert code==403
    code,_,_=request('POST','/api/v1/auth/login',{'username':'admin','password':'incorrect'})
    assert code==401
    code,body,setcookie=request('POST','/api/v1/auth/login',{'username':'admin','password':c['password']})
    assert code==200
    parsed=SimpleCookie();parsed.load(setcookie);item=parsed['netmonorc_session']
    assert item['secure'] and item['httponly'] and item['samesite']=='Strict' and item['path']=='/'
    cookie='netmonorc_session='+item.value
    view=json.loads(body);assert view['role']=='admin' and view['csrfToken'] and view['scopes']
    code,_,_=request('GET','/api/v1/monitors/research-web/assessment');assert code==200
    code,body,_=request('GET','/api/v1/monitors/research-web/observations?limit=2')
    assert code==200
    history=json.loads(body);assert len(history['items'])==2 and history['nextCursor']
    first={o['observationId'] for o in history['items']}
    from urllib.parse import quote
    code,body,_=request('GET','/api/v1/observations/'+quote(history['items'][0]['observationId'],safe=''))
    assert code==200 and json.loads(body)==history['items'][0]
    code,body,_=request('GET','/api/v1/monitors/research-web/observations?limit=2&cursor='+quote(history['nextCursor']))
    assert code==200 and not first.intersection(o['observationId'] for o in json.loads(body)['items'])
    code,body,_=request('GET','/api/v1/monitors/research-web/observations?zone=office&limit=2')
    assert code==200 and all(o['networkZone']=='office' for o in json.loads(body)['items'])
    change={'currentPassword':c['password'],'newPassword':c['replacement']}
    code,_,_=request('POST','/api/v1/auth/change-password',change);assert code==403
    code,body,setcookie=request('POST','/api/v1/auth/change-password',change,csrf=view['csrfToken']);assert code==200
    parsed=SimpleCookie();parsed.load(setcookie);rotated='netmonorc_session='+parsed['netmonorc_session'].value
    changed=json.loads(body);assert changed['csrfToken']!=view['csrfToken']
    code,_,_=request('GET','/api/v1/auth/me');assert code==401
    cookie=rotated
    code,_,_=request('POST','/api/v1/auth/login',{'username':'admin','password':c['password']});assert code==401
    code,body,setcookie=request('POST','/api/v1/auth/login',{'username':'admin','password':c['replacement']});assert code==200
    parsed=SimpleCookie();parsed.load(setcookie);cookie='netmonorc_session='+parsed['netmonorc_session'].value
    view=json.loads(body)
    code,body,own_tag=request('GET','/api/v1/users/'+view['userId'],response_header='ETag');assert code==200 and own_tag
    assert 'password' not in body.decode() and 'hash' not in body.decode()
    code,_,_=request('PATCH','/api/v1/users/'+view['userId'],{'disabled':True},csrf=view['csrfToken'],if_match=own_tag);assert code==409
    code,body,user_tag=request('POST','/api/v1/users',{'username':'live-viewer','role':'viewer','initialPassword':c['viewerPassword']},csrf=view['csrfToken'],response_header='ETag');assert code==201 and user_tag
    created=json.loads(body);assert created['role']=='viewer' and 'initialPassword' not in created
    admin_cookie=cookie;cookie=''
    code,body,setcookie=request('POST','/api/v1/auth/login',{'username':'live-viewer','password':c['viewerPassword']});assert code==200
    viewer_view=json.loads(body);parsed=SimpleCookie();parsed.load(setcookie);viewer_cookie='netmonorc_session='+parsed['netmonorc_session'].value
    cookie=viewer_cookie
    code,_,_=request('GET','/api/v1/users');assert code==403
    code,body,_=request('POST','/api/v1/auth/tokens',{'name':'live-viewer-reader','scopes':['resources:read']},csrf=viewer_view['csrfToken']);assert code==201
    viewer_token=json.loads(body)['token']
    code,_,_=request('GET','/api/v1/agents',bearer=viewer_token);assert code==403
    code,_,_=request('GET','/api/v1/diagnostics',bearer=viewer_token);assert code==403
    code,_,_=request('GET','/api/v1/alerts',bearer=viewer_token);assert code==403
    code,_,_=request('GET','/api/v1/webhook/deliveries',bearer=viewer_token);assert code==403
    code,_,_=request('POST','/api/v1/alerts/missing/acknowledge',{},bearer=viewer_token);assert code==403
    code,body,_=request('GET','/api/v1/dashboard/summary',bearer=viewer_token);assert code==200
    limited=json.loads(body);assert 'monitorStates' in limited and all(k not in limited for k in ['agentCounts','firingAlerts','recentDiagnostics'])
    cookie=admin_cookie
    code,_,_=request('PATCH','/api/v1/users/'+created['id'],{'disabled':True},csrf=view['csrfToken']);assert code==428
    code,body,new_tag=request('PATCH','/api/v1/users/'+created['id'],{'disabled':True},csrf=view['csrfToken'],if_match=user_tag,response_header='ETag');assert code==200 and json.loads(body)['disabled']
    code,_,_=request('PATCH','/api/v1/users/'+created['id'],{'disabled':False},csrf=view['csrfToken'],if_match=user_tag);assert code==412
    code,_,_=request('PATCH','/api/v1/users/'+created['id'],{'disabled':False},csrf=view['csrfToken'],if_match=new_tag);assert code==200
    code,_,_=request('GET','/api/v1/auth/me',bearer=viewer_token);assert code==401
    cookie=viewer_cookie
    code,_,_=request('GET','/api/v1/auth/me');assert code==401
    cookie=admin_cookie
    code,body,_=request('POST','/api/v1/auth/tokens',{'name':'live-cli','scopes':['resources:read','resources:write','agents:read','diagnostics:read','diagnostics:run','alerts:read','alerts:manage'],'expiresAt':None},csrf=view['csrfToken'])
    assert code==201
    api=json.loads(body);assert api['token'].startswith('nmo_pat_') and api['expiresAt'] is None
    code,body,_=request('GET','/api/v1/auth/me',bearer=api['token']);assert code==200
    assert json.loads(body)['expiresAt'] is None and 'csrfToken' not in json.loads(body)
    code,_,_=request('GET','/api/v1/monitors/research-web/assessment',bearer=api['token']);assert code==200
    code,_,_=request('GET','/api/v1/monitors/research-web/observations?limit=2',bearer=api['token']);assert code==200
    # Returned secrets go only to the parent operator process over a private
    # captured pipe, for the restart portion; never printed by the helper UI.
    print(json.dumps({'cookie':cookie,'csrf':view['csrfToken'],'userId':view['userId'],'apiToken':api['token'],'apiTokenId':api['id']}))
elif c['mode']=='configuration':
    cookie=c['cookie']
    code,body,etag=request('GET','/api/v1/resources/Monitor/research-web',response_header='ETag');assert code==200 and etag
    resource=json.loads(body);original=resource['manifest']
    import copy
    manifest=copy.deepcopy(c.get('manifest',original))
    if 'manifest' not in c:manifest['spec']['checks'][0]['config']['expectedStatus']=[503]
    code,body,_=request('POST','/api/v1/config/validate',{'manifests':[manifest]},csrf=c['csrf']);assert code==200 and json.loads(body)['valid']
    code,body,_=request('POST','/api/v1/config/plan',{'manifests':[manifest]},csrf=c['csrf']);assert code==200 and json.loads(body)['planId']
    import uuid
    batch={'manifests':[manifest],'expectedVersions':{'Monitor/research-web':resource['resourceVersion']},'requestId':str(uuid.uuid4())}
    if c.get('prepareOnly'):
        preview=json.loads(body)
        print(json.dumps({'revision':preview['diffs'][0]['effectiveRevision'],'original':original,'batch':batch}));sys.exit(0)
    code,body,_=request('POST','/api/v1/config/apply',batch,csrf=c['csrf']);assert code==202
    op=json.loads(body);assert op['state']=='accepted' and op['result']['desiredCommitted']
    print(json.dumps({'revision':op['result']['targets'][0]['effectiveRevision'],'original':original,'operationId':op['id'],'batch':batch}))
elif c['mode']=='control-prepare':
    cookie=c['cookie']
    code,_,_=request('POST','/api/v1/resources/DiagnosticProfile',c['profile'],csrf=c['csrf']);assert code==201
    code,_,_=request('POST','/api/v1/monitors/research-web/diagnostics',{'profileRef':'manual-slow','requestId':'no-csrf'});assert code==403
    print(json.dumps({'prepared':True}))
elif c['mode']=='control-disabled':
    cookie=c['cookie']
    code,_,_=request('POST','/api/v1/monitors/research-web/diagnostics',{'profileRef':'web-lab','requestId':'disabled-manual'},csrf=c['csrf']);assert code==409
    print(json.dumps({'disabledRejected':True}))
elif c['mode']=='webhook-prepare':
    cookie=c['cookie']
    code,_,_=request('POST','/api/v1/webhook/test',{});assert code==403
    code,_,_=request('POST','/api/v1/webhook/test',{'message':'not allowed'},csrf=c['csrf']);assert code==400
    print(json.dumps({'validated':True}))
elif c['mode']=='ack-prepare':
    cookie=c['cookie']
    code,body,tag=request('GET','/api/v1/alerts/'+c['alertId'],response_header='ETag');assert code==200 and tag
    assert json.loads(body)['state']=='firing'
    code,_,_=request('POST','/api/v1/alerts/'+c['alertId']+'/acknowledge',{},if_match=tag);assert code==403
    code,_,_=request('POST','/api/v1/alerts/'+c['alertId']+'/acknowledge',{},csrf=c['csrf']);assert code==428
    print(json.dumps({'etag':tag}))
elif c['mode']=='ack-stale':
    cookie=c['cookie']
    code,_,_=request('POST','/api/v1/alerts/'+c['alertId']+'/acknowledge',{},csrf=c['csrf'],if_match=c['etag']);assert code==412
    print(json.dumps({'staleRejected':True}))
elif c['mode']=='dependency-invalid':
    cookie=c['cookie']
    code,_,_=request('POST','/api/v1/resources/Dependency',c['manifest'],csrf=c['csrf']);assert code==422
    code,_,_=request('GET','/api/v1/resources/Dependency/'+c['manifest']['metadata']['name']);assert code==404
    print(json.dumps({'cycleRejected':True}))
elif c['mode']=='dependency-delete-guard':
    cookie=c['cookie']
    route='/api/v1/resources/Target/context-resolver'
    code,_,tag=request('GET',route,response_header='ETag');assert code==200
    code,_,_=request('DELETE',route,csrf=c['csrf'],if_match=tag);assert code==409
    print(json.dumps({'referenceProtected':True}))
elif c['mode']=='group-read':
    cookie=c['cookie']
    code,body,_=request('GET','/api/v1/resource-groups/'+c['name']+'/assessment');assert code==200
    print(body.decode())
elif c['mode']=='assessment-check':
    cookie=c['cookie']
    code,body,_=request('GET','/api/v1/monitors/research-web/assessment');assert code==200
    a=json.loads(body)
    assert a['revision']==c['revision'] and a['state']==c['state'] and a['coverageState']==c['coverage']
    assert {z['zone']:z['state'] for z in a['zoneStates']}==c['zones']
    print(json.dumps({'assessmentVerified':True}))
elif c['mode']=='rule-delete':
    cookie=c['cookie']
    route='/api/v1/resources/'+c.get('kind','AlertRule')+'/'+c['name']
    import time
    for attempt in range(20):
        try:
            code,body,tag=request('GET',route,response_header='ETag')
            if code not in [429,503]:break
        except (OSError,http.client.HTTPException):
            if attempt==19:raise
        time.sleep(.5)
    assert code==200 and tag
    code,_,_=request('DELETE',route,csrf=c['csrf'],if_match=tag);assert code==204
    # A Dependency delete may reconcile the active plan after its committed 204.
    # Retry only the safe confirmation read, never the already accepted DELETE.
    import time
    for attempt in range(20):
        try:
            code,_,_=request('GET',route)
            break
        except (OSError,http.client.HTTPException):
            if attempt==19:raise
            time.sleep(.5)
    assert code==404
    print(json.dumps({'deleted':True}))
elif c['mode']=='profile':
    cookie=c['cookie']
    code,body,_=request('GET','/api/v1/resources/DiagnosticProfile/web-lab');assert code==200
    resource=json.loads(body);original=resource['manifest']
    import copy,uuid
    manifest=copy.deepcopy(c.get('manifest',original))
    if 'manifest' not in c:manifest['spec']['actions'][0]['costUnits']=2
    batch={'manifests':[manifest],'expectedVersions':{'DiagnosticProfile/web-lab':resource['resourceVersion']},'requestId':str(uuid.uuid4())}
    code,body,_=request('POST','/api/v1/config/plan',batch,csrf=c['csrf']);assert code==200
    preview=json.loads(body);dependents=[d for d in preview['diffs'] if d['kind']=='Monitor' and d['name']=='research-web']
    assert len(dependents)==1 and dependents[0]['action']=='reconcile'
    print(json.dumps({'revision':dependents[0]['effectiveRevision'],'original':original,'batch':batch}))
elif c['mode']=='operation':
    cookie=c['cookie']
    code,body,_=request('GET','/api/v1/operations/'+c['operationId']);assert code==200
    op=json.loads(body)
    if 'batch' in c:
        code,replay,_=request('POST','/api/v1/config/apply',c['batch'],csrf=c['csrf']);assert code==202 and json.loads(replay)['id']==op['id']
    print(json.dumps({'state':op['state'],'finishedAt':op['finishedAt'],'allTargetsReady':all(t['ready'] for t in op['result']['targets'])}))
else:
    cookie=c['cookie']
    code,body,_=request('GET','/api/v1/auth/me');assert code==200
    view=json.loads(body);assert view['userId']==c['userId'] and view['csrfToken']==c['csrf']
    code,_,_=request('GET','/api/v1/auth/me',bearer=c['apiToken']);assert code==200
    code,body,_=request('GET','/api/v1/auth/tokens');assert code==200 and c['apiToken'] not in body.decode()
    code,_,_=request('DELETE','/api/v1/auth/tokens/'+c['apiTokenId'],csrf=c['csrf']);assert code==204
    code,_,_=request('GET','/api/v1/auth/me',bearer=c['apiToken']);assert code==401
    code,body,_=request('GET','/api/v1/agents');assert code==200
    agent_page=json.loads(body);assert len(agent_page['items'])==2
    assert all(a['onlineState']=='online' and a['appliedRevision'] and a['lastHeartbeat'] and a['certExpiresAt'] and a['maxConcurrentProbes']==2 and 0<=a['activeProbes']<=2 for a in agent_page['items'])
    assert all('token' not in a and 'privateKey' not in a for a in agent_page['items'])
    code,body,_=request('GET','/api/v1/agents?zone=office');assert code==200 and len(json.loads(body)['items'])==1
    code,body,_=request('GET','/api/v1/agents/branch');assert code==200 and json.loads(body)['networkZone']=='office'
    code,_,_=request('GET','/api/v1/agents/not-in-roster');assert code==404
    code,_,_=request('POST','/api/v1/auth/logout');assert code==403
    code,_,_=request('GET','/api/v1/auth/me');assert code==200
    code,_,_=request('POST','/api/v1/auth/logout',csrf=c['csrf']);assert code==204
    code,_,_=request('GET','/api/v1/auth/me');assert code==401
    print(json.dumps({'restartSessionRestored':True,'logoutRequiresCSRF':True,'revokedCookieRejected':True,'role':view['role'],'scopes':view['scopes']}))
'''

def verify(lab, placement_only=False, zones_only=False, groups_only=False, dependencies_only=False, dynamic_only=False):
    lab.require()
    if lab.sql('SELECT count(*) FROM nmo_users')!='0':
        raise RuntimeError('session smoke requires a fresh disposable project; existing identities are preserved')
    password=secrets.token_urlsafe(32)
    args=('exec','-T','controller','/usr/local/bin/netmonorc-admin','-database-file','/run/netmonorc/database.dsn','-username','admin','-password-stdin')
    lab.compose(*args,input=password.encode())
    # Re-running bootstrap must not replace credentials.
    lab.compose(*args,input=secrets.token_urlsafe(32).encode())
    common={'address':lab.prefix+'.40.4','ca':(lab.work/'pki/ca.crt').read_text()}
    first=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,'mode':'login','password':password,'replacement':secrets.token_urlsafe(32),'viewerPassword':secrets.token_urlsafe(24)}).encode()))
    cli=('/usr/local/bin/netmonorc',)
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    identity=json.loads(lab.compose('exec','-T','controller',*cli,'whoami','--context-file','/tmp/cli/context.json'))
    assert identity['userId']==first['userId'] and identity['expiresAt'] is None
    if any((placement_only,zones_only,groups_only,dependencies_only,dynamic_only)):
        import importlib
        cases=[(dynamic_only,'lab_dynamic','dynamic-placement-only'),(placement_only,'lab_placement','fixed-placement-only'),(zones_only,'lab_zones','zone-assessment-only'),(groups_only,'lab_groups','resource-groups-only'),(dependencies_only,'lab_dependencies','dependencies-only')]
        _,module,scope=next(case for case in cases if case[0])
        placement_result=importlib.import_module(module).verify(lab,cli,common,first,CLIENT)
        lab.compose('exec','-T','controller',*cli,'logout','--context-file','/tmp/cli/context.json')
        result=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'restart'}).encode()))
        result.update(placement_result)
        result.update({'status':'passed','scope':scope,'trustedTLS':True,'browserInteractionTested':False,'credentialsDiscarded':True})
        return result
    validation=json.loads(lab.compose('exec','-T','controller',*cli,'validate','--remote','--context-file','/tmp/cli/context.json','-f','/run/netmonorc/monitor.yaml'))
    assert validation['valid']
    def cli_configuration(manifest=None):
        args={**common,**first,'mode':'configuration','prepareOnly':True}
        if manifest is not None:args['manifest']=manifest
        prepared=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps(args).encode()))
        preview=json.loads(lab.compose('exec','-T','controller',*cli,'plan','--context-file','/tmp/cli/context.json','-f','-',input=json.dumps(prepared['batch']).encode()))
        assert preview['planId']
        op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(prepared['batch']).encode()))
        assert op['state']=='completed' and all(t['ready'] for t in op['result']['targets'])
        prepared['operationId']=op['id']
        return prepared
    password=''
    changed=cli_configuration()
    lab.wait(lambda:lab.assessment().get('state')=='down' and lab.assessment().get('revision')==changed['revision'],'updated probe plan reached Agents and new assessment')
    def operation_status(admission,replay=False):
        args={**common,**first,'mode':'operation','operationId':admission['operationId']}
        if replay:args['batch']=admission['batch']
        return json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps(args).encode()))
    lab.wait(lambda:operation_status(changed)['state']=='completed','apply operation completed after Agent acknowledgements')
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='down' and lab.assessment().get('revision')==changed['revision'],'persisted desired configuration survived process restart')
    recovered=operation_status(changed,True);assert recovered['state']=='completed' and recovered['allTargetsReady'] and recovered['finishedAt']
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    stored=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','operations',changed['operationId']))
    assert stored['state']=='completed' and stored['id']==changed['operationId']
    acknowledged_id=None
    def firing_alert():
        nonlocal acknowledged_id
        rows=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','alerts'))['items']
        if len(rows)==1 and rows[0]['state']=='firing':acknowledged_id=rows[0]['id'];return True
        return False
    lab.wait(firing_alert,'real active alert ready for operator acknowledgement')
    prepared_ack=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'ack-prepare','alertId':acknowledged_id}).encode()))
    before_outbox=int(lab.sql('SELECT count(*) FROM nmo_webhook_outbox'))
    acknowledged=json.loads(lab.compose('exec','-T','controller',*cli,'alerts','acknowledge','--context-file','/tmp/cli/context.json','--note-file','-','--if-match',prepared_ack['etag'],acknowledged_id,input=b'Integration reviewed'))
    assert acknowledged['state']=='firing' and acknowledged['acknowledgedAt'] and 'note' not in acknowledged
    assert lab.assessment()['state']=='down' and int(lab.sql('SELECT count(*) FROM nmo_webhook_outbox'))==before_outbox
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='down','fresh down assessment after acknowledged restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    receipt=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','alerts',acknowledged_id))
    assert receipt['acknowledgedAt']==acknowledged['acknowledgedAt'] and receipt['state']=='firing'
    restored=cli_configuration(changed['original'])
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==restored['revision'],'restored Monitor reconciled to healthy')
    lab.wait(lambda:operation_status(restored)['state']=='completed','restored apply confirmed by Agents')
    assert lab.sql("SELECT count(*) FROM nmo_assessments a JOIN nmo_observations o ON o.monitor=a.monitor WHERE a.monitor='research-web' AND o.revision<>a.revision AND o.id IN (SELECT jsonb_array_elements_text(z->'evidence') FROM jsonb_array_elements(a.payload->'zones') z)")=='0'

    import copy
    before=int(lab.sql('SELECT count(*) FROM nmo_diagnostic_runs'))
    disabled=copy.deepcopy(changed['original']);disabled['spec']['diagnostics']['enabled']=False
    off=cli_configuration(disabled)
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==off['revision'],'diagnostics disabled with regular checks preserved')
    checked=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'control-disabled'}).encode()));assert checked['disabledRejected']
    lab.inject('http')
    lab.wait(lambda:lab.assessment().get('state')=='down' and lab.assessment().get('revision')==off['revision'],'fault assessed while diagnostics disabled')
    assert int(lab.sql('SELECT count(*) FROM nmo_diagnostic_runs'))==before
    lab.inject('healthy');lab.wait(lambda:lab.assessment().get('state')=='healthy','regular service recovered with diagnostics disabled')
    enabled=cli_configuration(changed['original'])
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==enabled['revision'],'diagnostics re-enabled through apply')
    lab.inject('http')
    lab.wait(lambda:int(lab.sql('SELECT count(*) FROM nmo_diagnostic_runs'))>before,'new automatic diagnostic started after re-enable')
    def unacknowledged_cycle():
        alert=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','alerts',acknowledged_id))
        return alert['state']=='firing' and alert['acknowledgedAt'] is None and alert['firedAt']!=acknowledged['firedAt']
    lab.wait(unacknowledged_cycle,'new firing requires a new acknowledgement')
    stale=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'ack-stale','alertId':acknowledged_id,'etag':prepared_ack['etag']}).encode()));assert stale['staleRejected']

    lab.inject('healthy');lab.wait(lambda:lab.assessment().get('state')=='healthy','service recovered after re-enabled diagnostic')
    # Profile-only apply must wait for the unchanged dependent Monitor manifest
    # to receive a new execution revision and actual Agent acknowledgements.
    def cli_profile(manifest=None):
        args={**common,**first,'mode':'profile'}
        if manifest is not None:args['manifest']=manifest
        prepared=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps(args).encode()))
        op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(prepared['batch']).encode()))
        assert op['state']=='completed' and len(op['result']['targets'])==2
        assert all(t['ready'] for t in op['result']['targets'])
        assert any(t['kind']=='Monitor' and t.get('implicit') for t in op['result']['targets'])
        return prepared
    def get_monitor():
        return json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','Monitor','research-web'))
    original_monitor=get_monitor()
    updated_profile=cli_profile()
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==updated_profile['revision'],'profile-only change reconciled dependent Monitor')
    dependency=get_monitor()
    assert dependency['resourceVersion']==original_monitor['resourceVersion'] and dependency['generation']==original_monitor['generation']
    stored_profile=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','DiagnosticProfile','web-lab'))
    assert stored_profile['manifest']['spec']['actions'][0]['costUnits']==2
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('revision')==updated_profile['revision'] and lab.assessment().get('state')=='healthy','database profile survives Controller restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    recovered_profile=cli_profile(updated_profile['original'])
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==recovered_profile['revision'],'original profile restored through apply')
    def agent_status():
        return json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','agents','branch'))
    assert agent_status()['onlineState']=='online'
    lab.compose('stop','branch')
    lab.wait(lambda:agent_status()['onlineState']=='offline','Agent snapshot reports a disconnected stream')
    lab.compose('start','branch')
    lab.wait(lambda:agent_status()['onlineState']=='online','Agent snapshot follows fenced reconnect and heartbeat')
    lab.wait(lambda:lab.assessment().get('state')=='healthy','regular monitoring restored after Agent reconnect')
    sessions=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','diagnostics'))
    assert sessions['items']
    candidates=[s for s in sessions['items'] if s['budgetUsed']['additionalChecks']>0]
    assert candidates
    diagnostic=candidates[0]
    detail=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','diagnostics',diagnostic['id']))
    assert detail['monitorId']=='research-web' and detail['profileRef']=='web-lab'
    tasks=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','--tasks','diagnostics',diagnostic['id']))
    assert tasks['items'] and len(tasks['items'])==detail['budgetUsed']['additionalChecks']
    assert all(t['sessionId']==diagnostic['id'] and t['attemptNo']>=1 and 'check_spec' not in t for t in tasks['items'])
    assert all(observation in detail['evidenceIds'] for t in tasks['items'] for observation in t['observationIds'])
    profile={'apiVersion':'netmonorc.io/v1alpha1','kind':'DiagnosticProfile','metadata':{'name':'manual-slow'},'spec':{
        'mode':'fixed','budget':{'maxAdditionalChecks':1,'maxDuration':'60s','maxParallel':1},
        'actions':[{'id':'slow-http','check':{'type':'http','runner':'native-http','timeout':'15s','config':{'url':'http://198.18.0.80:8080/slow','expectedStatus':[200]}},'placement':{'agentRefs':['dc'],'zones':['server'],'strategy':'fixed'}}],
        'fixed':{'steps':[{'id':'slow-step','actionRef':'slow-http'}]}}}
    prepared=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'control-prepare','profile':profile}).encode()));assert prepared['prepared']
    request_id='manual-'+secrets.token_hex(12)
    runs_before=int(lab.sql('SELECT count(*) FROM nmo_diagnostic_runs'))
    import subprocess
    admitted=None
    def manual_start():
        nonlocal admitted
        try:
            admitted=json.loads(lab.compose('exec','-T','controller',*cli,'diagnostics','run','--context-file','/tmp/cli/context.json','--profile','manual-slow','--request-id',request_id,'research-web'))
            return True
        except subprocess.CalledProcessError:return False
    lab.wait(manual_start,'bounded manual run admitted after healthy/cooldown')
    assert admitted['mode']=='fixed' and admitted['profileRef']=='manual-slow'
    run_id=admitted['id']
    lab.wait(lambda:lab.sql("SELECT count(*) FROM nmo_diagnostic_tasks WHERE run_id='"+run_id+"' AND state='accepted'")=='1','slow physical probe accepted by Agent')
    cancelled=json.loads(lab.compose('exec','-T','controller',*cli,'diagnostics','cancel','--context-file','/tmp/cli/context.json',run_id))
    assert cancelled['state']=='cancelled' and cancelled['budgetUsed']['additionalChecks']==1
    lab.wait(lambda:lab.sql("SELECT count(*) FROM nmo_observations o JOIN nmo_diagnostic_tasks t ON t.id=o.diagnostic_task_id WHERE t.run_id='"+run_id+"'")=='1','TaskCancel stops the 12s probe before its normal response',seconds=8)
    assert lab.sql("SELECT state FROM nmo_diagnostic_tasks WHERE run_id='"+run_id+"'")=='cancelled'
    assert int(lab.sql('SELECT count(*) FROM nmo_diagnostic_runs'))==runs_before+1
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='healthy','fresh regular assessment after cancel/restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    def agents_rejoined():
        agents=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','agents'))['items']
        return len(agents)==2 and all(a['onlineState']=='online' and a['appliedRevision'] and a['lastHeartbeat'] and a['maxConcurrentProbes']==2 for a in agents)
    lab.wait(agents_rejoined,'Agent heartbeat and revision proof restored after manual-replay restart')
    assert manual_start() and admitted['id']==run_id and admitted['state']=='cancelled'
    assert int(lab.sql('SELECT count(*) FROM nmo_diagnostic_runs'))==runs_before+1
    alert_snapshot=None
    alert_events=None
    def alert_history_ready():
        nonlocal alert_snapshot,alert_events
        values=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','alerts'))['items']
        assert len(values)==1 and values[0]['monitorId']=='research-web' and values[0]['severity']=='critical'
        alert_snapshot=values[0]
        alert_events=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','--history','alerts',alert_snapshot['id']))['items']
        return alert_snapshot['state']=='resolved' and any(e['to']=='firing' for e in alert_events) and any(e['to']=='resolved' for e in alert_events)
    lab.wait(alert_history_ready,'API/CLI expose real firing and resolved transitions')
    detail=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','alerts',alert_snapshot['id']))
    assert detail['id']==alert_snapshot['id'] and detail['firedAt'] and detail['resolvedAt']
    deliveries=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','deliveries'))['items']
    assert deliveries and any(d['state']=='delivered' and d['attempts']>=1 and d['lastHttpStatus']==200 and d['lastAttemptAt'] for d in deliveries)
    encoded=json.dumps(deliveries)
    assert all(value not in encoded for value in ['https://','payload','lease_token','secretRef'])
    assert int(lab.sql('SELECT count(*) FROM nmo_webhook_outbox'))==len(deliveries)
    checked=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'webhook-prepare'}).encode()));assert checked['validated']
    lab.wait(lambda:lab.sql("SELECT count(*) FROM nmo_webhook_outbox WHERE state IN ('pending','leased')")=='0','ordinary notifications drained before isolated test')
    alerts_before=int(lab.sql('SELECT count(*) FROM nmo_alerts'))
    events_before=int(lab.sql('SELECT count(*) FROM nmo_alert_events'))
    mode={'generation':secrets.token_hex(8),'fail_next':1}
    lab.compose('exec','-T','webhook','python3','-c',"from pathlib import Path;import sys;p=Path('/data/mode.json');t=p.with_suffix('.tmp');t.write_text(sys.stdin.read());t.replace(p)",input=json.dumps(mode).encode())
    test_op=json.loads(lab.compose('exec','-T','controller',*cli,'webhook','test','--context-file','/tmp/cli/context.json','--wait','--timeout','45s'))
    assert test_op['kind']=='webhook.test' and test_op['state']=='completed' and test_op['result']['attempts']==2 and test_op['result']['lastHttpStatus']==200
    test_event=test_op['result']['eventId']
    check_receiver="import sqlite3,json,sys;db=sqlite3.connect('/data/receiver.sqlite');event=json.loads(db.execute('SELECT payload FROM events WHERE id=?',(sys.argv[1],)).fetchone()[0]);print(json.dumps({'valid':set(event)=={'event_id','event_type','occurred_at','schema_version'} and event['event_type']=='webhook.test' and event['schema_version']=='1','attempts':db.execute('SELECT count(*) FROM attempts WHERE event_id=?',(sys.argv[1],)).fetchone()[0]}))"
    receiver=json.loads(lab.compose('exec','-T','webhook','python3','-c',check_receiver,test_event));assert receiver['valid'] and receiver['attempts']==2
    assert int(lab.sql('SELECT count(*) FROM nmo_alerts'))==alerts_before and int(lab.sql('SELECT count(*) FROM nmo_alert_events'))==events_before
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='healthy','monitoring healthy after webhook operation restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    recovered_test=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','operations',test_op['id']))
    assert recovered_test['kind']=='webhook.test' and recovered_test['state']=='completed' and recovered_test['result']['eventId']==test_event
    lab.wait(agents_rejoined,'Agent proofs restored after webhook-test restart')
    overview=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','dashboard'))
    assert overview['monitorStates']['healthy']==1 and overview['coverageStates']['sufficient']==1 and overview['agentCounts']['online']==2 and overview['firingAlerts']==0 and overview['recentDiagnostics'] and overview['observedAt']
    evidence_id=lab.sql("SELECT id FROM nmo_observations WHERE diagnostic_task_id='' ORDER BY received_at DESC LIMIT 1").strip()
    evidence=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','observations',evidence_id))
    assert evidence['observationId']==evidence_id and evidence['configRevision'] and evidence['receivedAt']
    # Desired AlertRules are evaluated by the running Controller without Agent reconnect.
    import copy,uuid
    saved_rule=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','AlertRule','service-outage'))
    changed_rule=copy.deepcopy(saved_rule['manifest']);changed_rule['spec']['description']='managed rule runtime verification'
    rule_batch={'manifests':[changed_rule],'expectedVersions':{'AlertRule/service-outage':saved_rule['resourceVersion']},'requestId':str(uuid.uuid4())}
    rule_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(rule_batch).encode()))
    assert rule_op['state']=='completed' and rule_op['result']['targets'][0]['reason']=='alert_rule_evaluated'
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='healthy','fresh assessment after managed rule restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    lab.wait(agents_rejoined,'Agent proofs restored after managed rule restart')
    persisted_rule=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','AlertRule','service-outage'))
    assert persisted_rule['manifest']['spec']['description']=='managed rule runtime verification'
    extra_rule=copy.deepcopy(saved_rule['manifest']);extra_rule['metadata']['name']='secondary-rule';extra_rule['spec']['when']='false'
    extra_batch={'manifests':[extra_rule],'requestId':str(uuid.uuid4())}
    extra_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(extra_batch).encode()))
    assert extra_op['state']=='completed'
    assert lab.sql("SELECT payload->>'status' FROM nmo_alerts WHERE logical_key='secondary-rule/research-web/monitor'").strip()=='normal'
    deleted=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'rule-delete','name':'secondary-rule'}).encode()));assert deleted['deleted']
    restore_batch={'manifests':[saved_rule['manifest']],'expectedVersions':{'AlertRule/service-outage':persisted_rule['resourceVersion']},'requestId':str(uuid.uuid4())}
    restore_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(restore_batch).encode()));assert restore_op['state']=='completed'
    original_endpoint=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','WebhookEndpoint','default-webhook'))
    changed_endpoint=copy.deepcopy(original_endpoint['manifest']);changed_endpoint['spec']['timeout']='3s'
    standby_endpoint=copy.deepcopy(original_endpoint['manifest']);standby_endpoint['metadata']['name']='standby-webhook';standby_endpoint['spec']['enabled']=False
    endpoint_batch={'manifests':[standby_endpoint,changed_endpoint],'expectedVersions':{'WebhookEndpoint/default-webhook':original_endpoint['resourceVersion']},'requestId':str(uuid.uuid4())}
    endpoint_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(endpoint_batch).encode()))
    assert endpoint_op['state']=='completed' and all(t['reason']=='webhook_configuration_loaded' for t in endpoint_op['result']['targets'])
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='healthy','fresh assessment after webhook configuration restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    lab.wait(agents_rejoined,'Agent proofs restored after webhook configuration restart')
    persisted_endpoint=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','WebhookEndpoint','default-webhook'))
    assert persisted_endpoint['manifest']['spec']['timeout']=='3s'
    endpoint_test=json.loads(lab.compose('exec','-T','controller',*cli,'webhook','test','--context-file','/tmp/cli/context.json','--wait','--timeout','45s'))
    assert endpoint_test['state']=='completed' and endpoint_test['result']['lastHttpStatus']==200
    deleted_endpoint=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'rule-delete','kind':'WebhookEndpoint','name':'standby-webhook'}).encode()));assert deleted_endpoint['deleted']
    endpoint_restore={'manifests':[original_endpoint['manifest']],'expectedVersions':{'WebhookEndpoint/default-webhook':persisted_endpoint['resourceVersion']},'requestId':str(uuid.uuid4())}
    restored_endpoint=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(endpoint_restore).encode()));assert restored_endpoint['state']=='completed'
    lab.wait(agents_rejoined,'Agent heartbeat and revision proofs restored after final endpoint configuration')
    before_target=get_monitor()
    linked_monitor=copy.deepcopy(before_target['manifest'])
    target_manifest={'apiVersion':'netmonorc.io/v1alpha1','kind':'Target','metadata':{'name':'research-target'},'spec':{'type':'service','address':linked_monitor['spec']['target']['address'],'networkZone':'target-location','criticality':3}}
    del linked_monitor['spec']['target'];linked_monitor['spec']['targetRef']='research-target'
    linked_batch={'manifests':[linked_monitor,target_manifest],'expectedVersions':{'Monitor/research-web':before_target['resourceVersion']},'requestId':str(uuid.uuid4())}
    linked_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(linked_batch).encode()))
    assert linked_op['state']=='completed' and len(linked_op['result']['targets'])==2
    linked_revision=next(t['effectiveRevision'] for t in linked_op['result']['targets'] if t['kind']=='Monitor')
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==linked_revision,'targetRef plan executed by Agents')
    linked_source=get_monitor()
    target_resource=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','Target','research-target'))
    changed_target=copy.deepcopy(target_manifest);changed_target['spec']['criticality']=4
    target_only={'manifests':[changed_target],'expectedVersions':{'Target/research-target':target_resource['resourceVersion']},'requestId':str(uuid.uuid4())}
    target_preview=json.loads(lab.compose('exec','-T','controller',*cli,'plan','--context-file','/tmp/cli/context.json','-f','-',input=json.dumps(target_only).encode()))
    assert any(d['kind']=='Monitor' and d['action']=='reconcile' for d in target_preview['diffs'])
    target_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(target_only).encode()))
    assert target_op['state']=='completed' and any(t.get('implicit') and t['kind']=='Monitor' and t['reason']=='required_agents_applied' for t in target_op['result']['targets'])
    target_revision=next(t['effectiveRevision'] for t in target_op['result']['targets'] if t['kind']=='Monitor')
    assert target_revision!=linked_revision and get_monitor()['resourceVersion']==linked_source['resourceVersion']
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==target_revision,'target-only dependency revision assessment')
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='healthy','fresh targetRef assessment after restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    lab.wait(agents_rejoined,'targetRef Agent proofs after restart')
    persisted_target=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','Target','research-target'))
    assert persisted_target['manifest']['spec']['criticality']==4 and get_monitor()['manifest']['spec']['targetRef']=='research-target' and lab.assessment()['revision']==target_revision
    unlink_batch={'manifests':[before_target['manifest']],'expectedVersions':{'Monitor/research-web':linked_source['resourceVersion']},'requestId':str(uuid.uuid4())}
    unlinked=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(unlink_batch).encode()));assert unlinked['state']=='completed'
    deleted_target=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'rule-delete','kind':'Target','name':'research-target'}).encode()));assert deleted_target['deleted']
    lab.wait(agents_rejoined,'Agent proofs restored after target unlink')
    before_checks=get_monitor()
    check_template=copy.deepcopy(before_checks['manifest']['spec']['checks'])
    http_check=next(check for check in check_template if check['id']=='http')
    original_url=http_check['config']['url'];http_check['config']['url']='${params.url}';http_check['config']['expectedStatus']=['${params.expected}']
    check_profile={'apiVersion':'netmonorc.io/v1alpha1','kind':'CheckProfile','metadata':{'name':'regular-checks'},'spec':{'parameters':[{'name':'url','type':'url','required':True},{'name':'expected','type':'integer','default':200},{'name':'ratio','type':'float','default':1.0}],'checks':check_template}}
    profiled_monitor=copy.deepcopy(before_checks['manifest']);del profiled_monitor['spec']['checks']
    profiled_monitor['spec']['useProfiles']=[{'ref':'regular-checks','as':'base','parameters':{'url':original_url}}]
    for check in profiled_monitor['spec']['assessment']['checks']:check['id']='base__'+check['id']
    checks_batch={'manifests':[profiled_monitor,check_profile],'expectedVersions':{'Monitor/research-web':before_checks['resourceVersion']},'requestId':str(uuid.uuid4())}
    checks_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(checks_batch).encode()));assert checks_op['state']=='completed'
    check_revision=next(t['effectiveRevision'] for t in checks_op['result']['targets'] if t['kind']=='Monitor')
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==check_revision,'CheckProfile generated probes executed by Agents')
    profiled_source=get_monitor()
    current_checks=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','CheckProfile','regular-checks'))
    changed_checks=copy.deepcopy(check_profile);changed_checks['spec']['parameters'][1]['default']=503
    changed_batch={'manifests':[changed_checks],'expectedVersions':{'CheckProfile/regular-checks':current_checks['resourceVersion']},'requestId':str(uuid.uuid4())}
    changed_checks_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(changed_batch).encode()))
    assert changed_checks_op['state']=='completed' and any(t.get('implicit') and t['kind']=='Monitor' and t['reason']=='required_agents_applied' for t in changed_checks_op['result']['targets'])
    failed_revision=next(t['effectiveRevision'] for t in changed_checks_op['result']['targets'] if t['kind']=='Monitor')
    lab.wait(lambda:lab.assessment().get('state')=='down' and lab.assessment().get('revision')==failed_revision,'CheckProfile-only default change alters real probe assessment')
    assert get_monitor()['resourceVersion']==profiled_source['resourceVersion']
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='down','persisted CheckProfile assessment after restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    lab.wait(agents_rejoined,'CheckProfile Agent proofs after restart')
    persisted_checks=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','CheckProfile','regular-checks'))
    assert persisted_checks['manifest']['spec']['parameters'][1]['default']==503 and get_monitor()['manifest']['spec']['useProfiles'][0]['ref']=='regular-checks'
    restore_checks={'manifests':[check_profile],'expectedVersions':{'CheckProfile/regular-checks':persisted_checks['resourceVersion']},'requestId':str(uuid.uuid4())}
    restored_checks=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(restore_checks).encode()));assert restored_checks['state']=='completed'
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==check_revision,'restored profile assessment')
    unlink_checks={'manifests':[before_checks['manifest']],'expectedVersions':{'Monitor/research-web':profiled_source['resourceVersion']},'requestId':str(uuid.uuid4())}
    unlinked_checks=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(unlink_checks).encode()));assert unlinked_checks['state']=='completed'
    deleted_checks=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'rule-delete','kind':'CheckProfile','name':'regular-checks'}).encode()));assert deleted_checks['deleted']
    lab.wait(agents_rejoined,'Agent proofs restored after CheckProfile unlink')
    before_policy=get_monitor()
    policy_manifest={'apiVersion':'netmonorc.io/v1alpha1','kind':'AssessmentPolicy','metadata':{'name':'regular-assessment'},'spec':copy.deepcopy(before_policy['manifest']['spec']['assessment'])}
    policy_monitor=copy.deepcopy(before_policy['manifest']);del policy_monitor['spec']['assessment'];policy_monitor['spec']['assessmentRef']='regular-assessment'
    policy_batch={'manifests':[policy_monitor,policy_manifest],'expectedVersions':{'Monitor/research-web':before_policy['resourceVersion']},'requestId':str(uuid.uuid4())}
    policy_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(policy_batch).encode()));assert policy_op['state']=='completed'
    policy_revision=next(t['effectiveRevision'] for t in policy_op['result']['targets'] if t['kind']=='Monitor')
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==policy_revision,'shared AssessmentPolicy evaluated by Controller')
    policy_source=get_monitor()
    current_policy=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','AssessmentPolicy','regular-assessment'))
    changed_policy=copy.deepcopy(policy_manifest);changed_policy['spec']['rules']=[{'id':'policy-test','priority':100,'when':'true','then':{'state':'degraded','reason':'SHARED_POLICY_TEST'}}]
    policy_only={'manifests':[changed_policy],'expectedVersions':{'AssessmentPolicy/regular-assessment':current_policy['resourceVersion']},'requestId':str(uuid.uuid4())}
    changed_policy_op=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(policy_only).encode()))
    assert changed_policy_op['state']=='completed' and any(t.get('implicit') and t['kind']=='Monitor' and t['reason']=='required_agents_applied' for t in changed_policy_op['result']['targets'])
    degraded_revision=next(t['effectiveRevision'] for t in changed_policy_op['result']['targets'] if t['kind']=='Monitor')
    lab.wait(lambda:lab.assessment().get('state')=='degraded' and lab.assessment().get('revision')==degraded_revision,'policy-only update changes live assessment')
    assert get_monitor()['resourceVersion']==policy_source['resourceVersion']
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='degraded','persisted policy assessment after restart')
    lab.compose('exec','-T','controller',*cli,'login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file','/tmp/cli/context.json','--token-stdin',input=first['apiToken'].encode())
    lab.wait(agents_rejoined,'policy-bound Agent proofs after restart')
    persisted_policy=json.loads(lab.compose('exec','-T','controller',*cli,'get','--context-file','/tmp/cli/context.json','AssessmentPolicy','regular-assessment'))
    assert persisted_policy['manifest']['spec']['rules'][0]['then']['state']=='degraded' and get_monitor()['manifest']['spec']['assessmentRef']=='regular-assessment'
    restore_policy={'manifests':[policy_manifest],'expectedVersions':{'AssessmentPolicy/regular-assessment':persisted_policy['resourceVersion']},'requestId':str(uuid.uuid4())}
    restored_policy=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(restore_policy).encode()));assert restored_policy['state']=='completed'
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==policy_revision,'restored shared policy assessment')
    unlink_policy={'manifests':[before_policy['manifest']],'expectedVersions':{'Monitor/research-web':policy_source['resourceVersion']},'requestId':str(uuid.uuid4())}
    unlinked_policy=json.loads(lab.compose('exec','-T','controller',*cli,'apply','--context-file','/tmp/cli/context.json','-f','-','--wait','--timeout','45s',input=json.dumps(unlink_policy).encode()));assert unlinked_policy['state']=='completed'
    deleted_policy=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'rule-delete','kind':'AssessmentPolicy','name':'regular-assessment'}).encode()));assert deleted_policy['deleted']
    final_revision=next(t['effectiveRevision'] for t in unlinked_policy['result']['targets'] if t['kind']=='Monitor')
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==final_revision,'healthy original configuration after policy unlink')
    lab.wait(agents_rejoined,'Agent proofs restored after AssessmentPolicy unlink')
    from lab_placement import verify as verify_placement
    placement_result=verify_placement(lab,cli,common,first,CLIENT)
    lab.compose('exec','-T','controller',*cli,'logout','--context-file','/tmp/cli/context.json')
    result=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'restart'}).encode()))
    result.update(placement_result)
    result.update({'status':'passed','trustedTLS':True,'secureHttpOnlySameSiteCookie':True,'bootstrapIdempotent':True,'loginCrossOriginRejected':True,'wrongPasswordRejected':True,'cookieReadsAssessment':True,'passwordChangeRotatesSession':True,'patSurvivesRestartAndRevokes':True,'cliLoginWhoamiLogout':True,'userCreateRBACAndETag':True,'monitorCRUDAndReconciliation':True,'atomicApplyReplayAndCompletion':True,'cliRemoteValidatePlanApplyWait':True,'cliGetOperation':True,'assessmentPolicyRESTAndCLI':True,'policyOnlyApplyChangesAssessment':True,'policyDependencyPreservesManifestVersion':True,'assessmentPolicySurvivesRestart':True,'checkProfileGeneratedProbesAndCLI':True,'checkProfileOnlyApplyChangesAssessment':True,'checkProfileDependencyPreservesManifestVersion':True,'checkProfileSurvivesRestart':True,'targetReferenceRESTAndCLI':True,'targetOnlyApplyWaitsForDependentACK':True,'targetDependencyPreservesManifestVersion':True,'targetReferenceSurvivesRestart':True,'managedWebhookEndpointCRUDAndApply':True,'managedWebhookSurvivesRestart':True,'managedWebhookUsesPersistedSender':True,'managedAlertRuleApplyAndRuntimeEvaluation':True,'managedAlertRuleSurvivesRestart':True,'multipleAlertRulesAndDelete':True,'observationDetailRESTAndCLI':True,'observationDetailMatchesHistory':True,'dashboardRESTAndCLI':True,'dashboardScopesIsolated':True,'dashboardUsesFreshAssessmentAndAgentProofs':True,'webhookTestThroughDurableOutbox':True,'webhookTestExactPayloadNoMessage':True,'webhookTestRetriesSameEvent':True,'webhookTestDoesNotCreateAlert':True,'webhookTestOperationSurvivesRestart':True,'alertAcknowledgementCLIAndAPI':True,'acknowledgementSurvivesRestart':True,'acknowledgementDoesNotResolveOrNotify':True,'newFiringClearsAcknowledgement':True,'staleAcknowledgementRejected':True,'acknowledgementScopeAndCSRF':True,'alertHistoryRESTAndCLI':True,'alertFiringResolvedLifecycleVisible':True,'webhookDeliveryRESTAndCLI':True,'deliveryViewOmitsSecrets':True,'alertReadScopeEnforced':True,'manualDiagnosticStartAndCancel':True,'taskCancelStopsPhysicalProbe':True,'manualStartReplaySurvivesRestart':True,'disabledManualStartRejected':True,'diagnosticHistoryRESTAndCLI':True,'diagnosticPhysicalTasksAndBudget':True,'diagnosticReadScopeEnforced':True,'profileCRUDAndCLI':True,'profileOnlyApplyWaitsForDependentACK':True,'profileDependencyPreservesManifestVersion':True,'databaseProfileSurvivesRestart':True,'profileRestoreViaApply':True,'agentSessionAPIAndCLI':True,'agentDisconnectReconnectSnapshot':True,'agentReadScopeEnforced':True,'diagnosticToggleViaApply':True,'disabledDiagnosticsSuppressRuns':True,'reEnabledDiagnosticsTriggerRuns':True,'operationReadyAfterAgentACK':True,'desiredConfigSurvivesRestart':True,'liveEvidenceRevisionIsolated':True,'disabledCredentialsStayRevoked':True,'lastAdminProtected':True,'oldPasswordRejected':True,'historyPaginationAndZoneFilter':True,'browserInteractionTested':False,'credentialsDiscarded':True})
    return result

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',default='172.28');p.add_argument('--output',default='.local/session-live.json');group=p.add_mutually_exclusive_group();group.add_argument('--placement-only',action='store_true',help='fixed placement workflow on a fresh stand');group.add_argument('--zones-only',action='store_true',help='zone SLA workflow on a fresh stand');group.add_argument('--groups-only',action='store_true',help='resource group workflow on a fresh stand');group.add_argument('--dynamic-only',action='store_true',help='dynamic regular placement workflow on a fresh stand');group.add_argument('--dependencies-only',action='store_true',help='dependency context workflow on a fresh stand');a=p.parse_args()
    try:
        result=verify(Lab(a.project,a.prefix),a.placement_only,a.zones_only,a.groups_only,a.dependencies_only,a.dynamic_only)
    except __import__('subprocess').CalledProcessError as e:
        raise SystemExit('Integration subprocess failed: '+e.stderr.decode(errors='replace')[-1200:])
    import os
    target=Path(a.output);target.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as f:json.dump(result,f,indent=2);f.write('\n')
    print('Verified '+result.get('scope','HTTPS identity/resources/CLI integration')+'; secrets omitted.')
