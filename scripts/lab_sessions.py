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
def request(method,path,body=None,csrf='',origin='https://localhost'):
    global cookie
    data=json.dumps(body).encode() if body is not None else b''
    headers={'Host':'localhost','Connection':'close','Content-Length':str(len(data))}
    if body is not None:headers['Content-Type']='application/json'
    if cookie:headers['Cookie']=cookie
    if csrf:headers['X-CSRF-Token']=csrf
    if origin:headers['Origin']=origin
    with socket.create_connection((c['address'],8443),timeout=8) as raw:
        with ctx.wrap_socket(raw,server_hostname='localhost') as sock:
            sock.sendall((method+' '+path+' HTTP/1.1\r\n'+''.join(k+': '+v+'\r\n' for k,v in headers.items())+'\r\n').encode()+data)
            response=http.client.HTTPResponse(sock);response.begin();payload=response.read(16384)
            return response.status,payload,response.getheader('Set-Cookie')
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
    # Returned secrets go only to the parent operator process over a private
    # captured pipe, for the restart portion; never printed by the helper UI.
    print(json.dumps({'cookie':cookie,'csrf':view['csrfToken'],'userId':view['userId']}))
else:
    cookie=c['cookie']
    code,body,_=request('GET','/api/v1/auth/me');assert code==200
    view=json.loads(body);assert view['userId']==c['userId'] and view['csrfToken']==c['csrf']
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
    first=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,'mode':'login','password':password}).encode()))
    password=''
    previous=lab.assessment()['at'];lab.compose('restart','controller')
    lab.wait(lambda:lab.assessment().get('at','')>previous and lab.assessment().get('state')=='healthy','fresh assessment after restart')
    result=json.loads(lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps({**common,**first,'mode':'restart'}).encode()))
    result.update({'status':'passed','trustedTLS':True,'secureHttpOnlySameSiteCookie':True,'bootstrapIdempotent':True,'loginCrossOriginRejected':True,'wrongPasswordRejected':True,'cookieReadsAssessment':True,'browserInteractionTested':False,'credentialsDiscarded':True})
    return result

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',default='172.28');p.add_argument('--output',default='.local/session-live.json');a=p.parse_args()
    result=verify(Lab(a.project,a.prefix))
    import os
    target=Path(a.output);target.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    fd=os.open(target,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as f:json.dump(result,f,indent=2);f.write('\n')
    print('Verified real HTTPS login, persistent session, CSRF logout and revoked cookie rejection; secrets omitted.')
