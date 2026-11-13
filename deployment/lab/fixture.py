"""Isolated fault target. Controls are files, never commands from network input."""
import json
from pathlib import Path
import socket
import socketserver
import struct
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

STATE = Path('/state/mode.json')

def mode():
    return json.loads(STATE.read_text())

class HTTP(BaseHTTPRequestHandler):
    def do_GET(self):
        try:
            state = mode()
            body = b'queue_depth=0 ready=true\n'
            self.send_response(state['http_status'])
            self.send_header('Content-Length', str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        except (BrokenPipeError, ConnectionResetError):
            pass
    def log_message(self, *_):
        pass

class TCP(socketserver.BaseRequestHandler):
    def handle(self):
        self.request.sendall(b'netmonorc-lab\n')

class DNS(socketserver.BaseRequestHandler):
    def handle(self):
        data, sock = self.request
        # Only the stand's bounded IN/A query is accepted.
        if not 17 <= len(data) <= 512 or data[4:6] != b'\x00\x01':
            return
        end = 12
        while end < len(data) and data[end]:
            length = data[end]
            if length > 63:
                return
            end += length + 1
        if end+5 > len(data) or data[end+1:end+5] != b'\x00\x01\x00\x01':
            return
        failure = mode()['dns_mode'] == 'nxdomain'
        answer = b'' if failure else b'\xc0\x0c'+struct.pack('!HHIH',1,1,30,4)+socket.inet_aton('198.18.0.80')
        response = data[:2]+struct.pack('!HHHHH',0x8183 if failure else 0x8180,1,0 if failure else 1,0,0)+data[12:end+5]+answer
        sock.sendto(response,self.client_address)

class Listener(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True

http = ThreadingHTTPServer(('0.0.0.0',8080),HTTP)
# UDP replies must retain the virtual source IP on this multihomed target.
dns = socketserver.ThreadingUDPServer(('198.18.0.80',1053),DNS)
for server in (http,dns):
    threading.Thread(target=server.serve_forever,daemon=True).start()
print('Isolated target listening: HTTP 8080, TCP 9090, DNS 1053',flush=True)
tcp = None
while True:
    enabled = mode()['tcp_open']
    if enabled and tcp is None:
        tcp = Listener(('0.0.0.0',9090),TCP)
        threading.Thread(target=tcp.serve_forever,daemon=True).start()
    elif not enabled and tcp is not None:
        tcp.shutdown();tcp.server_close();tcp = None
    threading.Event().wait(.1)
