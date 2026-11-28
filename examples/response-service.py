#!/usr/bin/env python3
"""Dedicated demonstration content; the marker is synthetic, never a secret."""
import argparse
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--port', type=int, default=18088)
    parser.add_argument('--exposed-flag', default='/tmp/netmonorc-demo-exposed')
    args = parser.parse_args()
    flag = Path(args.exposed_flag)
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            body = ('exposure=' + ('public-debug' if flag.exists() else 'restricted') + '\n').encode()
            self.send_response(200)
            self.send_header('Content-Type', 'text/plain')
            self.send_header('Content-Length', str(len(body)))
            self.send_header('Cache-Control', 'no-store')
            self.end_headers()
            self.wfile.write(body)
    with ThreadingHTTPServer(('0.0.0.0', args.port), Handler) as server:
        try:
            server.serve_forever()
        except KeyboardInterrupt:
            pass

if __name__ == '__main__':
    main()
