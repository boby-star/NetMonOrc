"""Run the owned real admin UI acceptance; credentials stay in private .local files."""
import argparse,json,secrets,ssl,subprocess,urllib.request,time,http.client
from pathlib import Path
from http.cookiejar import CookieJar
from lab import Lab, ROOT

def verify(lab, output):
    output=Path(output).resolve();output.mkdir(mode=0o700,parents=True,exist_ok=True)
    private=output/'private.json'
    if private.exists():
        config=json.loads(private.read_text())
        if config.get('project')!=lab.project:raise RuntimeError('Private browser state belongs to another project')
    else:
        if lab.sql('SELECT count(*) FROM nmo_users')!='0':raise RuntimeError('Use a fresh owned project')
        config={'project':lab.project,'username':'design-admin','password':secrets.token_urlsafe(32),'originalPassword':'','newPassword':secrets.token_urlsafe(32),'viewerPassword':secrets.token_urlsafe(32)}
        config['originalPassword']=config['password'];private.write_text(json.dumps(config));private.chmod(0o600)
        subprocess.run(['python3',str(ROOT/'scripts/lab.py'),'admin','--project',lab.project,'--prefix',lab.prefix,'--username',config['username'],'--password-stdin'],input=config['password'].encode(),stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True)
    origin='https://localhost:'+str(lab.ui_port())
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPCookieProcessor(CookieJar()),urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=str(lab.work/'pki/ca.crt'))))
    def request(method,path,body=None,csrf=None,etag=None):
        headers={'Content-Type':'application/json'}
        if csrf:headers['X-CSRF-Token']=csrf
        if etag:headers['If-Match']=etag
        raw=json.dumps(body).encode() if body is not None else None
        for attempt in range(60):
            try:
                with opener.open(urllib.request.Request(origin+path,raw,headers,method=method),timeout=15) as r:
                    data=r.read();return json.loads(data) if data else None
            except urllib.error.HTTPError as e:
                if method!='GET' or e.code not in [429,503] or attempt==59:raise
                time.sleep(.5)
            except (OSError,http.client.HTTPException) as e:
                if method!='GET' or attempt==59:raise
                time.sleep(.5)
    try:request('GET','/api/v1/auth/me')
    except urllib.error.HTTPError as e:
        if e.code!=401:raise
    try:session=request('POST','/api/v1/auth/login',{'username':config['username'],'password':config['password']})
    except urllib.error.HTTPError as e:
        if e.code!=401:raise
        config['password']=config['newPassword']
        session=request('POST','/api/v1/auth/login',{'username':config['username'],'password':config['password']})
    config['originalPassword']=config['password'];config['newPassword']=secrets.token_urlsafe(32)
    config['viewerName']='design-viewer-'+secrets.token_hex(3);config['viewerPassword']=secrets.token_urlsafe(32);config['tokenName']='design-token-'+secrets.token_hex(3)
    private.write_text(json.dumps(config));private.chmod(0o600)
    def create(kind,name,spec):
        try:request('GET','/api/v1/resources/'+kind+'/'+name)
        except urllib.error.HTTPError as e:
            if e.code!=404:raise
            try:request('POST','/api/v1/resources/'+kind,{'apiVersion':'netmonorc.io/v1alpha1','kind':kind,'metadata':{'name':name},'spec':spec},session['csrfToken'])
            except http.client.HTTPException:
                request('GET','/api/v1/resources/'+kind+'/'+name)
            time.sleep(2)
            request('GET','/api/v1/auth/me')
    create('Target','design-dns',{'type':'logical'})
    create('Dependency','design-edge',{'from':{'kind':'Monitor','name':'research-web'},'to':{'kind':'Target','name':'design-dns'},'relation':'requires','strength':'soft'})
    create('ResourceGroup','design-group',{'members':['research-web'],'mode':'derived','evaluation':{'strategy':'all-required','minCoverage':1}})
    create('AlertRule','design-alert',{'monitorRef':'research-web','when':'monitor.state == "healthy"','resolveWhen':'monitor.state == "down"','for':'0s','cooldown':'0s','notifyEvents':[],'channelRef':'default-webhook'})
    existing=request('GET','/api/v1/alerts?monitor=research-web&limit=100')['items']
    if any(x['ruleRef']=='design-alert' and x.get('acknowledgedAt') for x in existing):
        path='/api/v1/resources/AlertRule/design-alert'
        for trigger,recover,wanted in [('false','monitor.state == "healthy"','resolved'),('monitor.state == "healthy"','monitor.state == "down"','firing')]:
            current=request('GET',path);manifest=current['manifest'];manifest['spec']['when']=trigger;manifest['spec']['resolveWhen']=recover
            request('PUT',path,manifest,session['csrfToken'],'"'+current['resourceVersion']+'"')
            lab.wait(lambda:lab.sql("SELECT payload->>'status' FROM nmo_alerts WHERE logical_key='design-alert/research-web/monitor'")==wanted,'fresh owned alert cycle')
    create('DiagnosticProfile','design-slow-v2',{'mode':'fixed','budget':{'maxAdditionalChecks':1,'maxDuration':'60s','maxParallel':1},'hypotheses':[{'id':'responding'},{'id':'broken'}],'actions':[{'id':'read','check':{'type':'http','runner':'native-http','timeout':'15s','config':{'url':'http://198.18.0.80:8080/slow','expectedStatus':[200]}},'placement':{'strategy':'fixed','agentRefs':['branch'],'zones':['office']},'expectedOutcomes':{'responding':'success','broken':'failure'}}],'fixed':{'steps':[{'id':'read','actionRef':'read'}]}})
    request('POST','/api/v1/auth/logout',csrf=session['csrfToken'])
    if not lab.diagnostic():
        try:
            lab.inject('http');lab.wait(lambda:lab.diagnostic() if (lab.diagnostic() or {}).get('finishedAt') else None,'completed real diagnostic',seconds=80)
        finally:lab.healthy()
    lab.wait(lambda:lab.assessment().get('state')=='healthy','healthy after fixtures')
    downloads=output/'downloads';downloads.mkdir(exist_ok=True)
    config.update({'origin':origin,'caFile':str(lab.work/'pki/ca.crt'),'output':str(output/'browser.json'),'screenshot':str(output/'desktop.png'),'mobileScreenshot':str(output/'mobile.png'),'failureScreenshot':str(output/'failure.png'),'downloadDir':str(downloads)})
    subprocess.run(['node',str(ROOT/'scripts/admin_ui_browser.mjs')],input=json.dumps(config).encode(),check=True,timeout=480)

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',required=True);p.add_argument('--output',required=True);a=p.parse_args()
    verify(Lab(a.project,a.prefix),a.output)
