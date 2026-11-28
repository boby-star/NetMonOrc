#!/usr/bin/env python3
"""Opt-in Linux nftables helper. Unix socket only; no arbitrary commands."""
import argparse
import fcntl
import ipaddress
import json
import os
import re
import socket
import socketserver
import struct
import subprocess
import tempfile
import sys
import time
from http.server import BaseHTTPRequestHandler
from pathlib import Path

TABLE = 'netmonorc_response'
OWNER = 'NetMonOrc bounded response v1'
PROTECTED = {5432, 8443, 9443}


def source(value):
    net = ipaddress.ip_network(value, strict=True)
    ip = net.network_address
    if net.prefixlen != net.max_prefixlen or ip.is_loopback or ip.is_multicast or ip.is_unspecified or getattr(ip, 'ipv4_mapped', None):
        raise ValueError('single non-loopback source host required')
    return str(net)


def nft(script):
    try:
        return subprocess.run(['nft', '-f', '-'], input=script, text=True,
                              capture_output=True, timeout=2, check=True)
    except subprocess.CalledProcessError as error:
        print('nft: ' + error.stderr[:1024], file=sys.stderr)
        raise


def setup():
    existing = subprocess.run(['nft', '-j', 'list', 'table', 'inet', TABLE],
                              text=True, capture_output=True, timeout=2)
    if existing.returncode == 0:
        objects = json.loads(existing.stdout)['nftables']
        owned = any(v.get('table', {}).get('comment') == OWNER for v in objects)
        sets = {v['set']['name'] for v in objects if 'set' in v}
        chains = {v['chain']['name'] for v in objects if 'chain' in v}
        if not owned or sets != {'blocked4', 'blocked6'} or chains != {'ingress'}:
            raise RuntimeError('existing nft table is not this helper schema')
        return
    interfaces = json.loads(subprocess.run(['ip', '-j', 'address', 'show'],
                            text=True, capture_output=True, timeout=2, check=True).stdout)
    addresses = [ipaddress.ip_address(a['local']) for interface in interfaces
                 for a in interface.get('addr_info', []) if 'local' in a]
    rules = ''
    for version, family, key in [(4, 'ip', 'blocked4'), (6, 'ip6', 'blocked6')]:
        local = sorted({str(ip) for ip in addresses if ip.version == version and not ip.is_loopback})
        if local:
            destinations = ', '.join(local)
            rules += f'add rule inet {TABLE} ingress {family} daddr {{ {destinations} }} {family} saddr . tcp dport @{key} drop\n'
    if not rules:
        raise RuntimeError('no non-loopback local address available')
    nft(f'''add table inet {TABLE} {{ comment "{OWNER}"; }}
add set inet {TABLE} blocked4 {{ type ipv4_addr . inet_service; flags timeout; }}
add set inet {TABLE} blocked6 {{ type ipv6_addr . inet_service; flags timeout; }}
add chain inet {TABLE} ingress {{ type filter hook prerouting priority -310; policy accept; }}
{rules}''')


class Policy:
    def __init__(self, ports, sources, state_file):
        self.ports = set(ports)
        self.sources = {source(s) for s in sources}
        if not self.ports or any(type(p) is not int or p < 1024 or p > 65535 or p in PROTECTED for p in self.ports) or not self.sources:
            raise ValueError('explicit unprivileged non-management ports and sources required')
        self.path = Path(state_file)
        self.path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        self.journal = json.loads(self.path.read_text()) if self.path.exists() else {}
        if not isinstance(self.journal, dict):
            raise ValueError('invalid response journal')

    def save(self):
        fd, name = tempfile.mkstemp(prefix='.response-', dir=self.path.parent)
        try:
            with os.fdopen(fd, 'w') as stream:
                json.dump(self.journal, stream, separators=(',', ':'))
                stream.flush()
                os.fsync(stream.fileno())
            os.replace(name, self.path)
            directory = os.open(self.path.parent, os.O_RDONLY | os.O_DIRECTORY)
            try:
                os.fsync(directory)
            finally:
                os.close(directory)
        finally:
            if os.path.exists(name):
                os.unlink(name)

    def block(self, request):
        if not isinstance(request, dict) or set(request) != {'id', 'port', 'sourceCIDR', 'untilUnixMilli'}:
            raise ValueError('invalid request fields')
        key, port, until = request['id'], request['port'], request['untilUnixMilli']
        if not isinstance(key, str) or not re.fullmatch('[0-9a-f]{32}', key):
            raise ValueError('invalid request id')
        if type(port) is not int or port not in self.ports or source(request['sourceCIDR']) not in self.sources:
            raise ValueError('response outside local allowlist')
        if type(until) is not int:
            raise ValueError('integer expiry required')
        now = int(time.time() * 1000)
        # Idempotency is checked before expiry; a retry never adds kernel time.
        prior = self.journal.get(key)
        if prior is not None:
            if prior['request'] != request:
                raise ValueError('id reused with different request')
            if until <= now:
                return {'status': 'rejected', 'reason': 'response_expired'}
            return prior['result']
        if until <= now or until - now > 300_000:
            raise ValueError('expiry must be within five minutes')
        self.journal = {k: v for k, v in self.journal.items()
                        if v['request']['untilUnixMilli'] > now - 3_600_000}
        if len(self.journal) >= 4096:
            raise ValueError('response journal full')
        # Reserve durably first; uncertainty cannot cause a replay or extension.
        entry = {'request': request, 'result': {'status': 'unacknowledged', 'reason': 'reserved_outcome_unknown'}}
        self.journal[key] = entry
        self.save()
        ip = ipaddress.ip_network(request['sourceCIDR']).network_address
        family = 'blocked4' if ip.version == 4 else 'blocked6'
        remaining = until - int(time.time() * 1000)
        if remaining <= 0:
            entry['result'] = {'status': 'rejected', 'reason': 'response_expired'}
        else:
            # Numeric address/port/timeout only; no shell and no user nft text.
            try:
                nft(f'add element inet {TABLE} {family} {{ {ip} . {port} timeout {remaining}ms }}\n')
                entry['result'] = {'status': 'applied', 'reason': 'kernel_timeout_installed'}
            except subprocess.TimeoutExpired:
                entry['result'] = {'status': 'unacknowledged', 'reason': 'nft_outcome_unknown'}
            except (OSError, subprocess.SubprocessError):
                entry['result'] = {'status': 'failed', 'reason': 'nft_apply_failed'}
        self.save()
        return entry['result']


def handler(policy, agent_uid):
    class Handler(BaseHTTPRequestHandler):
        def address_string(self):
            return 'local-agent'

        def do_POST(self):
            try:
                _, uid, _ = struct.unpack('3i', self.connection.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))
                if uid != agent_uid:
                    raise ValueError('peer uid not allowed')
                length = int(self.headers.get('Content-Length', '0'))
                if self.path != '/block' or not 0 < length <= 2048:
                    raise ValueError('invalid endpoint or body length')
                request = json.loads(self.rfile.read(length))
                result = policy.block(request)
            except (ValueError, TypeError, KeyError):
                result = {'status': 'rejected', 'reason': 'local_policy_rejected'}
            except OSError:
                result = {'status': 'unacknowledged', 'reason': 'journal_io_failed'}
            body = json.dumps(result).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return Handler


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--socket', default='/run/netmonorc-response/helper.sock')
    parser.add_argument('--state-file', default='/var/lib/netmonorc-response/journal.json')
    parser.add_argument('--ports', required=True)
    parser.add_argument('--sources', required=True)
    parser.add_argument('--agent-uid', type=int, default=10001)
    parser.add_argument('--socket-gid', type=int, default=10001)
    args = parser.parse_args()
    if os.geteuid() != 0:
        parser.error('run as root on the Agent host; Agent itself remains unprivileged')
    policy = Policy([int(p) for p in args.ports.split(',')], args.sources.split(','), args.state_file)
    lock = open(str(policy.path) + '.lock', 'a')
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    setup()
    path = Path(args.socket)
    path.parent.mkdir(mode=0o750, parents=True, exist_ok=True)
    os.chown(path.parent, 0, args.socket_gid)
    os.chmod(path.parent, 0o750)
    if path.exists():
        if not __import__('stat').S_ISSOCK(path.lstat().st_mode):
            raise RuntimeError('socket path is not a socket')
        path.unlink()
    with socketserver.UnixStreamServer(str(path), handler(policy, args.agent_uid)) as server:
        os.chown(path, 0, args.socket_gid)
        os.chmod(path, 0o660)
        server.timeout = 1
        # Limit an idle accepted connection independently of rule TTL.
        original = server.get_request
        def get_request():
            connection, address = original()
            connection.settimeout(3)
            return connection, address
        server.get_request = get_request
        try:
            server.serve_forever()
        finally:
            path.unlink(missing_ok=True)
    # No rule deletion on shutdown: kernel element timeouts restore access.


if __name__ == '__main__':
    main()
