"""Verify real HTTPS API and production React assets inside the private stand.

No host port, insecure TLS, token arguments or browser automation is used.
"""
import argparse
import json
from pathlib import Path
from lab import Lab

CLIENT = r'''
import http.client,json,socket,ssl,sys
from html.parser import HTMLParser
config=json.load(sys.stdin)
ctx=ssl.create_default_context(cadata=config['ca'])
ctx.minimum_version=ssl.TLSVersion.TLSv1_3
versions=[]
def get(path,authorized=True):
    with socket.create_connection((config['address'],8443),timeout=5) as raw:
        with ctx.wrap_socket(raw,server_hostname='localhost') as sock:
            versions.append(sock.version())
            auth=('Authorization: Bearer '+config['token']+'\r\n') if authorized else ''
            request='GET '+path+' HTTP/1.1\r\nHost: localhost\r\n'+auth+'Connection: close\r\n\r\n'
            sock.sendall(request.encode())
            response=http.client.HTTPResponse(sock);response.begin()
            return response.status,response.read(1048577)
status,body=get('/api/v1/monitors/research-web/assessment')
assert status==200,status
assessment=json.loads(body)
assert assessment['state']=='healthy' and assessment['coverageState']=='sufficient'
anonymous,_=get('/api/v1/monitors/research-web/assessment',False)
assert anonymous==401
status,body=get('/')
assert status==200 and b'NetMonOrc' in body
class Assets(HTMLParser):
    def __init__(self):super().__init__();self.paths=[]
    def handle_starttag(self,tag,attrs):
        for key,value in attrs:
            if key in ['src','href'] and value.startswith('/assets/'):self.paths.append(value)
assets=Assets();assets.feed(body.decode())
assert assets.paths
for path in assets.paths:
    status,body=get(path)
    assert status==200 and len(body)>0
print(json.dumps({'assessment':assessment,'anonymousStatus':anonymous,'productionAssets':len(assets.paths),'tlsVersions':versions,'browserInteractionTested':False}))
'''

def verify(lab):
    lab.require()
    payload={'address':lab.prefix+'.40.4','ca':(lab.work/'pki/ca.crt').read_text(),
             'token':(lab.work/'viewer-token').read_text().strip()}
    # office-net already belongs to control; only the operator tool receives
    # this stdin payload. It never prints the token or private configuration.
    raw=lab.compose('exec','-T','office-net','python3','-c',CLIENT,input=json.dumps(payload).encode())
    return json.loads(raw)

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project',default='netmonorc-lab')
    parser.add_argument('--prefix',default='172.30')
    parser.add_argument('--output',default='.local/rest-react.json')
    args=parser.parse_args()
    result=verify(Lab(args.project,args.prefix))
    path=Path(args.output);path.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
    import os
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as output:json.dump(result,output,indent=2);output.write('\n')
    print('Verified trusted HTTPS Assessment, 401 and production React assets; no host ports published.')
