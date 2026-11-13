"""Generate short-lived, private certificates for an isolated research stand."""
from pathlib import Path
import argparse
import os
import re
import subprocess

def issue_server(root, name, ip):
    """Issue a lab receiver identity; the existing CA key stays on the host."""
    import ipaddress
    root = Path(root)
    if not re.fullmatch('[a-z][a-z0-9-]{0,62}',name): raise ValueError('invalid identity')
    ip = str(ipaddress.ip_address(ip))
    def run(*args):
        subprocess.run(['openssl',*args],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.PIPE)
    run('req','-new','-newkey','rsa:2048','-nodes','-subj',f'/CN={name}',
        '-keyout',str(root/f'{name}.key'),'-out',str(root/f'{name}.csr'))
    ext = root/f'{name}.ext'
    ext.write_text('basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\n'
                   f'extendedKeyUsage=serverAuth\nsubjectAltName=IP:{ip}\n')
    run('x509','-req','-in',str(root/f'{name}.csr'),'-CA',str(root/'ca.crt'),
        '-CAkey',str(root/'ca.key'),'-CAcreateserial','-days','2','-extfile',str(ext),'-out',str(root/f'{name}.crt'))
    for suffix in ['key','csr','ext','crt']: (root/f'{name}.{suffix}').chmod(0o600)

def generate(root, agents=('branch', 'dc')):
    root = Path(root)
    root.mkdir(mode=0o700, parents=True, exist_ok=True)
    if any(root.iterdir()):
        raise RuntimeError('PKI directory must be empty; existing identities are preserved')
    def run(*args):
        subprocess.run(['openssl', *args], check=True, stdout=subprocess.DEVNULL,
                       stderr=subprocess.PIPE)
    run('req', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '2',
        '-subj', '/CN=NetMonOrc research CA', '-addext', 'basicConstraints=critical,CA:TRUE',
        '-addext', 'keyUsage=critical,keyCertSign,cRLSign',
        '-keyout', str(root/'ca.key'), '-out', str(root/'ca.crt'))
    for name in ['controller', *agents]:
        if not re.fullmatch('[a-z][a-z0-9-]{0,62}', name):
            raise ValueError('invalid certificate identity')
        key, csr, crt = (root/f'{name}.{suffix}' for suffix in ['key', 'csr', 'crt'])
        run('req', '-new', '-newkey', 'rsa:2048', '-nodes', '-subj', f'/CN={name}',
            '-keyout', str(key), '-out', str(csr))
        ext = root/f'{name}.ext'
        usage = 'serverAuth' if name == 'controller' else 'clientAuth'
        san = 'DNS:localhost,IP:127.0.0.1' if name == 'controller' else f'URI:spiffe://netmonorc/agent/{name}'
        ext.write_text(f'basicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage={usage}\nsubjectAltName={san}\n')
        run('x509', '-req', '-in', str(csr), '-CA', str(root/'ca.crt'),
            '-CAkey', str(root/'ca.key'), '-CAcreateserial', '-days', '2',
            '-extfile', str(ext), '-out', str(crt))
    for file in root.iterdir():
        os.chmod(file, 0o600)

if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('directory')
    args = parser.parse_args()
    generate(args.directory)
