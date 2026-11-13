"""Test ICMP in an owned isolated network namespace, never the host firewall."""
import argparse
import json
from pathlib import Path
import subprocess
import secrets

ROOT = Path(__file__).resolve().parents[1]
IMAGE = 'postgres:17.6-alpine@sha256:ef257d85f76e48da1c64832459b59fcaba1a4dac97bf5d7450c77753542eee94'
parser = argparse.ArgumentParser()
parser.add_argument('--blackbox-binary', required=True)
args = parser.parse_args()
config = ROOT/'.local/icmp-smoke.yaml'
config.write_text('''apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata:
  name: icmp-loopback
spec:
  target: {address: 127.0.0.1}
  checks:
    - id: icmp
      type: icmp
      timeout: 1s
      config: {}
  assessment:
    requiredZones: [office]
''')
(ROOT/'.local/netmonorc-static').chmod(0o755)
config.chmod(0o644)  # Contains no secrets; readable by the non-root test process.
common = ['docker', 'create', '--network', 'none',
          '--tmpfs', '/tmp:rw,nosuid,nodev,size=16m,mode=1777', '--cap-drop', 'ALL',
          '--entrypoint', '/netmonorc']
probe = ['probe', '-config', '/manifest.yaml', '-agent', 'branch', '-zone', 'office',
         '-allow-cidrs', '127.0.0.1/32', '-blackbox-binary', '/blackbox']
def run(extra):
    name = 'netmonorc-icmp-' + secrets.token_hex(4)
    subprocess.check_output(common + ['--name', name, *extra, IMAGE] + probe)
    try:
        # Copy files: a remote Docker daemon need not share workspace mounts.
        for source, dest in [(ROOT/'.local/netmonorc-static', '/netmonorc'),
                             (Path(args.blackbox_binary).resolve(), '/blackbox'),
                             (config, '/manifest.yaml')]:
            subprocess.run(['docker', 'cp', str(source), name+':'+dest], check=True)
        output = subprocess.check_output(['docker', 'start', '-a', name])
        code = subprocess.check_output(['docker', 'inspect', '-f', '{{.State.ExitCode}}', name]).strip()
        assert code == b'0', code
        return json.loads(output)
    finally:
        subprocess.run(['docker', 'rm', '-fv', name], check=True, stdout=subprocess.DEVNULL)

positive = run(['--cap-add', 'NET_RAW'])
assert positive['execution'] == 'completed' and positive['outcome'] == 'success', positive
print('PASS: real ICMP loopback with NET_RAW in an isolated container', flush=True)
negative = run(['--user', '65534:65534', '--sysctl', 'net.ipv4.ping_group_range=0 0'])
assert negative['execution'] == 'execution_failed' and negative['outcome'] == 'unknown', negative
print('PASS: no ICMP socket privileges -> execution_failed/unknown', flush=True)
