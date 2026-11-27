#!/usr/bin/env python3
"""Dedicated deterministic target; fault control is restricted to loopback.

No host firewall, production service or NetMonOrc process is changed.
The controller source of truth is the local /state endpoint, never the planner.
"""
import argparse
import ipaddress
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

FAULTS = {"healthy", "application-error", "backend-error",
          "office-access-restriction", "listener-unavailable"}


class Server(ThreadingHTTPServer):
    daemon_threads = True
    allow_reuse_address = True


class Fixture:
    def __init__(self, bind, port, office_ip, server_ip):
        if office_ip == server_ip:
            raise ValueError("office and server must have distinct source addresses")
        self.bind, self.port = bind, port
        self.office_ip, self.server_ip = office_ip, server_ip
        self.fault = "healthy"
        self.lock = threading.Lock()
        self.control_lock = threading.Lock()
        self.service = None

    def state(self):
        with self.lock:
            return {"fault": self.fault, "listener": self.service is not None}

    def set_fault(self, fault):
        if fault not in FAULTS:
            raise ValueError("unknown fault")
        with self.control_lock:
            if fault == "listener-unavailable":
                with self.lock:
                    service = self.service
                if service is not None:
                    service.shutdown()
                    service.server_close()
                with self.lock:
                    self.service = None
                    self.fault = fault
            else:
                with self.lock:
                    service = self.service
                if service is None:
                    service = Server((self.bind, self.port), self.service_handler())
                    threading.Thread(target=service.serve_forever,
                                     kwargs={"poll_interval": 0.05}, daemon=True).start()
                with self.lock:
                    self.service = service
                    self.fault = fault
        return self.state()

    def service_handler(self):
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                source = self.client_address[0]
                if source not in {fixture.office_ip, fixture.server_ip, "127.0.0.1"}:
                    return reply(self, 403, {"error": "source not allowed"})
                if self.path not in {"/ready", "/backend"}:
                    return reply(self, 404, {"error": "unknown endpoint"})
                fault = fixture.state()["fault"]
                denied = fault == "office-access-restriction" and source == fixture.office_ip
                failed = fault == "backend-error" or (
                    fault == "application-error" and self.path == "/ready")
                code = 403 if denied else (503 if failed else 200)
                reply(self, code, {"ready": code == 200, "endpoint": self.path})
        return Handler

    def control_handler(self):
        fixture = self

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                if self.path != "/state":
                    return reply(self, 404, {"error": "use /state"})
                reply(self, 200, fixture.state())

            def do_POST(self):
                if self.path != "/fault":
                    return reply(self, 404, {"error": "use /fault"})
                try:
                    length = int(self.headers.get("Content-Length", "0"))
                    if not 0 < length <= 1024:
                        raise ValueError("invalid content length")
                    value = json.loads(self.rfile.read(length))
                    if not isinstance(value, dict) or set(value) != {"fault"}:
                        raise ValueError("expected one fault field")
                    fault = value["fault"]
                    if not isinstance(fault, str):
                        raise ValueError("fault must be a string")
                    reply(self, 200, fixture.set_fault(fault))
                except (ValueError, TypeError) as error:
                    reply(self, 400, {"error": str(error)})
                except OSError as error:
                    reply(self, 409, {"error": str(error)})
        return Handler


def reply(handler, status, value):
    body = json.dumps(value).encode()
    handler.send_response(status)
    handler.send_header("Content-Type", "application/json")
    handler.send_header("Content-Length", str(len(body)))
    handler.send_header("Cache-Control", "no-store")
    handler.end_headers()
    handler.wfile.write(body)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bind", default="0.0.0.0")
    parser.add_argument("--service-port", type=int, default=18088)
    parser.add_argument("--admin-port", type=int, default=18089)
    parser.add_argument("--office-ip", default="188.137.224.35")
    parser.add_argument("--server-ip", default="188.137.254.31")
    args = parser.parse_args()
    for value in (args.office_ip, args.server_ip):
        if ipaddress.ip_address(value).version != 4:
            parser.error("this demonstration uses IPv4 source addresses")
    if args.service_port == args.admin_port:
        parser.error("service and control ports must differ")
    fixture = Fixture(args.bind, args.service_port, args.office_ip, args.server_ip)
    admin = Server(("127.0.0.1", args.admin_port), fixture.control_handler())
    try:
        fixture.set_fault("healthy")
        print(json.dumps({"service": f"{args.bind}:{args.service_port}",
                          "control": f"127.0.0.1:{args.admin_port}",
                          "office_source": args.office_ip,
                          "server_source": args.server_ip}), flush=True)
        admin.serve_forever(poll_interval=0.1)
    except KeyboardInterrupt:
        pass
    finally:
        admin.server_close()
        fixture.set_fault("listener-unavailable")


if __name__ == "__main__":
    main()
