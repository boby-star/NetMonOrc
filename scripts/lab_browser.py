"""Real UI acceptance on a fresh owned stand with explicit loopback HTTPS."""
import argparse
import json
from pathlib import Path
import secrets
import subprocess
import sys
from lab import Lab, ROOT

def verify(lab,output,screenshot):
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
    config={'origin':'https://localhost:'+str(port),'username':username,'password':password,'caFile':str(lab.work/'pki/ca.crt'),'output':str(output),'screenshot':str(screenshot)}
    subprocess.run(['node',str(ROOT/'scripts/live_browser_smoke.mjs')],input=json.dumps(config).encode(),check=True,timeout=240)
    result=json.loads(output.read_text());result['administratorBootstrapIdempotent']=True
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
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--project',required=True);p.add_argument('--prefix',default='172.19');p.add_argument('--output',default='.local/live-browser-r29.json');p.add_argument('--screenshot',default='.local/live-browser-r29.png');a=p.parse_args()
    try:verify(Lab(a.project,a.prefix),a.output,a.screenshot)
    except subprocess.CalledProcessError:raise SystemExit('Live browser acceptance did not pass; credentials were not exported')
    print('Verified real UI and loopback-only publishing; credentials omitted.')
