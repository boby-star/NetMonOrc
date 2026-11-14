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
    cookie=admin_cookie
    code,_,_=request('PATCH','/api/v1/users/'+created['id'],{'disabled':True},csrf=view['csrfToken']);assert code==428
    code,body,new_tag=request('PATCH','/api/v1/users/'+created['id'],{'disabled':True},csrf=view['csrfToken'],if_match=user_tag,response_header='ETag');assert code==200 and json.loads(body)['disabled']
    code,_,_=request('PATCH','/api/v1/users/'+created['id'],{'disabled':False},csrf=view['csrfToken'],if_match=user_tag);assert code==412
    code,_,_=request('PATCH','/api/v1/users/'+created['id'],{'disabled':False},csrf=view['csrfToken'],if_match=new_tag);assert code==200
    code,_,_=request('GET','/api/v1/auth/me',bearer=viewer_token);assert code==401
    cookie=viewer_cookie
    code,_,_=request('GET','/api/v1/auth/me');assert code==401
    cookie=admin_cookie
    code,body,_=request('POST','/api/v1/auth/tokens',{'name':'live-cli','scopes':['resources:read'],'expiresAt':None},csrf=view['csrfToken'])
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
    code,body,_=request('POST','/api/v1/config/apply',batch,csrf=c['csrf']);assert code==202
    op=json.loads(body);assert op['state']=='accepted' and op['result']['desiredCommitted']
    print(json.dumps({'revision':op['result']['targets'][0]['effectiveRevision'],'original':original,'operationId':op['id'],'batch':batch}))
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
    code,_,_=request('POST','/api/v1/auth/logout');assert code==403
    code,_,_=request('GET','/api/v1/auth/me');assert code==200
    code,_,_=request('POST','/api/v1/auth/logout',csrf=c['csrf']);assert code==204
    code,_,_=request('GET','/api/v1/auth/me');assert code==401
    print(json.dumps({'restartSessionRestored':True,'logoutRequiresCSRF':True,'revokedCookieRejected':True,'role':view['role'],'scopes':view['scopes']}))
'''

def verify(lab):
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
    lab.compose('exec','-T','controller',*cli,'logout','--context-file','/tmp/cli/context.json')
    password=''
    changed=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'configuration'}).encode()))
    lab.wait(lambda:lab.assessment().get('state')=='down' and lab.assessment().get('revision')==changed['revision'],'updated probe plan reached Agents and new assessment')
    def operation_status(admission,replay=False):
        args={**common,**first,'mode':'operation','operationId':admission['operationId']}
        if replay:args['batch']=admission['batch']
        return json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps(args).encode()))
    lab.wait(lambda:operation_status(changed)['state']=='completed','apply operation completed after Agent acknowledgements')
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='down' and lab.assessment().get('revision')==changed['revision'],'persisted desired configuration survived process restart')
    recovered=operation_status(changed,True);assert recovered['state']=='completed' and recovered['allTargetsReady'] and recovered['finishedAt']
    restored=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'configuration','manifest':changed['original']}).encode()))
    lab.wait(lambda:lab.assessment().get('state')=='healthy' and lab.assessment().get('revision')==restored['revision'],'restored Monitor reconciled to healthy')
    lab.wait(lambda:operation_status(restored)['state']=='completed','restored apply confirmed by Agents')
    assert lab.sql("SELECT count(*) FROM nmo_assessments a JOIN nmo_observations o ON o.monitor=a.monitor WHERE a.monitor='research-web' AND o.revision<>a.revision AND o.id IN (SELECT jsonb_array_elements_text(z->'evidence') FROM jsonb_array_elements(a.payload->'zones') z)")=='0'

    result=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'restart'}).encode()))
    result.update({'status':'passed','trustedTLS':True,'secureHttpOnlySameSiteCookie':True,'bootstrapIdempotent':True,'loginCrossOriginRejected':True,'wrongPasswordRejected':True,'cookieReadsAssessment':True,'passwordChangeRotatesSession':True,'patSurvivesRestartAndRevokes':True,'cliLoginWhoamiLogout':True,'userCreateRBACAndETag':True,'monitorCRUDAndReconciliation':True,'atomicApplyReplayAndCompletion':True,'operationReadyAfterAgentACK':True,'desiredConfigSurvivesRestart':True,'liveEvidenceRevisionIsolated':True,'disabledCredentialsStayRevoked':True,'lastAdminProtected':True,'oldPasswordRejected':True,'historyPaginationAndZoneFilter':True,'browserInteractionTested':False,'credentialsDiscarded':True})
    return result

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',default='172.28');p.add_argument('--output',default='.local/session-live.json');a=p.parse_args()
    result=verify(Lab(a.project,a.prefix))
    import os
    target=Path(a.output);target.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as f:json.dump(result,f,indent=2);f.write('\n')
    print('Verified real HTTPS identity/users/PAT/history, CLI, persisted sessions and revocation; secrets omitted.')
