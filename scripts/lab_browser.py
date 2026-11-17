"""Real UI acceptance on a fresh owned stand with explicit loopback HTTPS."""
import argparse
import json
from pathlib import Path
import secrets
import ssl
import subprocess
import sys
import urllib.request
from http.cookiejar import CookieJar
from lab import Lab, ROOT

def verify_cli(lab, username, password, report):
    origin='https://localhost:'+str(lab.ui_port())
    opener=urllib.request.build_opener(urllib.request.ProxyHandler({}),
        urllib.request.HTTPCookieProcessor(CookieJar()),
        urllib.request.HTTPSHandler(context=ssl.create_default_context(cafile=str(lab.work/'pki/ca.crt'))))
    def request(method,path,body=None,csrf=None):
        headers={'Content-Type':'application/json'}
        if csrf:headers['X-CSRF-Token']=csrf
        data=json.dumps(body).encode() if body is not None else None
        with opener.open(urllib.request.Request(origin+path,data=data,headers=headers,method=method),timeout=10) as response:
            raw=response.read(1048576)
            return json.loads(raw) if raw else None
    session=request('POST','/api/v1/auth/login',{'username':username,'password':password})
    token=request('POST','/api/v1/auth/tokens',{'name':'acceptance-cli','scopes':['resources:read','diagnostics:read','agents:read']},session['csrfToken'])
    context='/tmp/acceptance-cli/context.json'
    def cli(*args,input=None):
        return lab.compose('exec','-T','controller','/usr/local/bin/netmonorc',*args,input=input)
    try:
        cli('login','--server','https://localhost:8443','--ca-file','/run/netmonorc/ca.crt','--context-file',context,'--token-stdin',input=token['token'].encode())
        session_view=json.loads(cli('get','--context-file',context,'diagnostics',report['id']))
        assert session_view['id']==report['id'] and session_view['state']=='completed'
        tasks=json.loads(cli('get','--context-file',context,'--tasks','diagnostics',report['id']))['items']
        task=next(t for t in tasks if t['observationIds'])
        observation=json.loads(cli('get','--context-file',context,'observations',task['observationIds'][0]))
        assert observation['diagnosticSessionId']==report['id'] and observation['diagnosticTaskId']==task['id']
        assert observation['agentId']==task['agentId'] and observation['monitorId']=='research-web'
        assert len(json.loads(cli('get','--context-file',context,'agents'))['items'])==2
    finally:
        cli('logout','--context-file',context)
        request('DELETE','/api/v1/auth/tokens/'+token['id'],csrf=session['csrfToken'])
        request('POST','/api/v1/auth/logout',csrf=session['csrfToken'])

def verify(lab,output,screenshot,multi_monitor=False):
    lab.require()
    port=lab.ui_port()
    if not port:raise RuntimeError('Enable a loopback UI port with lab.py up --ui-port PORT first')
    if lab.sql('SELECT count(*) FROM nmo_users')!='0':raise RuntimeError('Live browser acceptance needs a fresh owned stand; existing users are preserved')
    output=Path(output).resolve();screenshot=Path(screenshot).resolve()
    if output.exists() or screenshot.exists():raise FileExistsError('Use new output and screenshot paths')
    output.parent.mkdir(parents=True,mode=0o700,exist_ok=True);screenshot.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    password=secrets.token_urlsafe(32);username='browser-admin'
    cmd=[sys.executable,str(ROOT/'scripts/lab.py'),'admin','--project',lab.project,'--prefix',lab.prefix,'--username',username,'--password-stdin']
    subprocess.run(cmd,input=password.encode(),stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True)
    preserved=subprocess.run(cmd,input=secrets.token_urlsafe(32).encode(),stdout=subprocess.PIPE,stderr=subprocess.PIPE,check=True).stdout
    if b'credentials unchanged' not in preserved:raise RuntimeError('Bootstrap idempotency not confirmed')
    # Create real diagnostic evidence through the normal Controller/Agent path.
    # Never insert synthetic tasks or observations into the database.
    previous=lab.diagnostic()
    previous_id=previous['id'] if previous else None
    try:
        lab.inject('http')
        def completed():
            report=lab.diagnostic()
            return report if report and report['id']!=previous_id and report.get('finishedAt') else None
        report=lab.wait(completed,'completed diagnostic after HTTP fault',seconds=60)
        if report['result']['status']!='completed' or report['result']['hypotheses']!=['http-failure']:
            raise RuntimeError('HTTP fault diagnostic did not match the injected fault')
    finally:
        lab.healthy()
    verify_cli(lab,username,password,report)
    config={'multiMonitor':multi_monitor,'origin':'https://localhost:'+str(port),'username':username,'password':password,'caFile':str(lab.work/'pki/ca.crt'),'output':str(output),'screenshot':str(screenshot)}
    config['diagnosticSessionId']=report['id']
    subprocess.run(['node',str(ROOT/'scripts/live_browser_smoke.mjs')],input=json.dumps(config).encode(),check=True,timeout=240)
    result=json.loads(output.read_text());result['administratorBootstrapIdempotent']=True
    result['cliDiagnosticTaskObservationProvenance']=True
    result['cliAgentSnapshots']=True
    if multi_monitor:
        counts=json.loads(lab.sql("SELECT jsonb_object_agg(monitor,counts) FROM (SELECT monitor,jsonb_build_object('agents',count(DISTINCT agent_id),'observations',count(*),'successes',count(*) FILTER(WHERE payload->>'outcome'='success'),'failures',count(*) FILTER(WHERE payload->>'outcome'='failure')) AS counts FROM nmo_observations WHERE diagnostic_task_id='' GROUP BY monitor) q"))
        if counts.get('research-web',{}).get('agents')!=2 or counts.get('research-other',{}).get('agents')!=2 or counts['research-other']['failures']<2:
            raise RuntimeError('Separate observations from both Agents were not confirmed')
        result['multipleMonitorObservationCounts']=counts

    bindings=json.loads(subprocess.check_output(['docker','inspect',lab.compose('ps','--quiet','controller').decode().strip()],text=True))[0]['HostConfig']['PortBindings']
    if bindings!={'8443/tcp':[{'HostIp':'127.0.0.1','HostPort':str(port)}]}:raise RuntimeError('Unexpected published service ports')
    containers=lab.compose('ps','--quiet').decode().split()
    inspected=json.loads(subprocess.check_output(['docker','inspect',*containers],text=True))
    for container in inspected:
        service=container['Config']['Labels']['com.docker.compose.service']
        if service!='controller' and container['HostConfig'].get('PortBindings'):
            raise RuntimeError('A private service unexpectedly publishes host ports')
        if service!='controller' and any(name.endswith('_ui-access') for name in container['NetworkSettings']['Networks']):
            raise RuntimeError('A private service unexpectedly joined the UI network')
    result['controllerOnlyLoopbackPublished']=True
    result['privateServicesExcludedFromUINetwork']=True
    output.write_text(json.dumps(result,indent=2)+'\n');output.chmod(0o600)

if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',default='172.19');p.add_argument('--output',default='.local/live-browser-r29.json');p.add_argument('--screenshot',default='.local/live-browser-r29.png');p.add_argument('--multi-monitor',action='store_true');a=p.parse_args()
    try:verify(Lab(a.project,a.prefix),a.output,a.screenshot,a.multi_monitor)
    except subprocess.CalledProcessError:raise SystemExit('Live browser acceptance did not pass; credentials were not exported')
    print('Verified real UI and loopback-only publishing; credentials omitted.')
