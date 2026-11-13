"""Tiny deterministic DNS fixtures for the stand; not a production probe engine."""
import socketserver
import struct
import threading

class DNSFixture:
    def __init__(self):
        self.mode = 'answers'
        self.requests = 0
        owner = self
        class UDP(socketserver.BaseRequestHandler):
            def handle(self):
                data, sock = self.request
                sock.sendto(owner.response(data), self.client_address)
        class TCP(socketserver.BaseRequestHandler):
            def read(self, n):
                b = b''
                while len(b) < n:
                    chunk = self.request.recv(n-len(b))
                    if not chunk:
                        raise EOFError('short DNS fixture request')
                    b += chunk
                return b
            def handle(self):
                self.request.settimeout(3)
                size = struct.unpack('!H', self.read(2))[0]
                response = owner.response(self.read(size))
                self.request.sendall(struct.pack('!H', len(response)) + response)
        self.udp = socketserver.UDPServer(('127.0.0.1', 0), UDP)
        self.port = self.udp.server_address[1]
        self.tcp = socketserver.TCPServer(('127.0.0.1', self.port), TCP)
        self.threads = []
        for s in [self.udp, self.tcp]:
            t = threading.Thread(target=s.serve_forever, daemon=True)
            t.start()
            self.threads.append(t)

    def response(self, data):
        self.requests += 1
        end = 12
        while data[end]:
            end += data[end] + 1
        question = data[12:end+5]
        rcode = 3 if self.mode == 'nxdomain' else 0
        answers = b''
        count = 0
        if self.mode == 'answers':
            for last in [10, 20]:
                answers += b'\xc0\x0c' + struct.pack('!HHIH', 1, 1, 60, 4) + bytes([127, 0, 0, last])
                count += 1
        return data[:2] + struct.pack('!HHHHH', 0x8180 | rcode, 1, count, 0, 0) + question + answers

    def close(self):
        for s in [self.udp, self.tcp]:
            s.shutdown()
            s.server_close()
        for t in self.threads:
            t.join()
