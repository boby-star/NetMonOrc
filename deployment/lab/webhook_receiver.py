"""Private research-only HTTPS receiver; durable event dedup and fault control.

No public management routes. A local private mode file selects how many
upcoming requests receive 503. Production receivers are external to NetMonOrc.
"""
import hmac
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import sqlite3
import ssl
import threading
from datetime import datetime, timezone

ROOT = Path('/run/netmonorc')
DATA = Path('/data')
TOKEN = (ROOT/'token').read_text().strip()
LOCK = threading.Lock()
DB = DATA/'receiver.sqlite'
with sqlite3.connect(DB) as db:
    db.executescript('''
    CREATE TABLE IF NOT EXISTS events(id TEXT PRIMARY KEY,payload TEXT NOT NULL);
    CREATE TABLE IF NOT EXISTS attempts(seq INTEGER PRIMARY KEY,event_id TEXT,status INTEGER,received_at TEXT);
    CREATE TABLE IF NOT EXISTS faults(generation TEXT PRIMARY KEY,used INTEGER NOT NULL);
    ''')

class Receiver(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass # No headers, bearer tokens or payloads enter container logs.

    def do_POST(self):
        if self.path != '/events' or not hmac.compare_digest(self.headers.get('Authorization',''), 'Bearer '+TOKEN):
            self.send_error(403); return
        try:
            size = int(self.headers.get('Content-Length','0'))
            if not 0 < size <= 1048576: raise ValueError('invalid size')
            event = json.loads(self.rfile.read(size))
            event_id = event['event_id']
            if self.headers.get('X-NetMonOrc-Event-ID') != event_id: raise ValueError('id mismatch')
            with LOCK, sqlite3.connect(DB) as db:
                mode = json.loads((DATA/'mode.json').read_text())
                generation = mode['generation']
                row = db.execute('SELECT used FROM faults WHERE generation=?',(generation,)).fetchone()
                used = row[0] if row else 0
                fail = used < mode['fail_next']
                db.execute('INSERT INTO faults VALUES(?,?) ON CONFLICT(generation) DO UPDATE SET used=excluded.used',(generation,used+1))
                status = 503 if fail else 200
                if not fail:
                    body = json.dumps(event,sort_keys=True,separators=(',',':'))
                    previous = db.execute('SELECT payload FROM events WHERE id=?',(event_id,)).fetchone()
                    if previous and previous[0] != body: raise ValueError('conflicting id')
                    db.execute('INSERT OR IGNORE INTO events VALUES(?,?)',(event_id,body))
                db.execute('INSERT INTO attempts(event_id,status,received_at) VALUES(?,?,?)',
                           (event_id,status,datetime.now(timezone.utc).isoformat()))
        except (ValueError, KeyError, OSError):
            self.send_error(400); return
        self.send_response(status)
        self.send_header('Content-Length','0')
        self.end_headers()

server = ThreadingHTTPServer(('0.0.0.0',9444),Receiver)
server.daemon_threads = True
server.timeout = 5
tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
tls.minimum_version = ssl.TLSVersion.TLSv1_3
tls.load_cert_chain(ROOT/'webhook.crt',ROOT/'webhook.key')
server.socket = tls.wrap_socket(server.socket,server_side=True)
server.serve_forever()
