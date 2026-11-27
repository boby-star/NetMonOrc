"""Real loopback HTTP/TCP checks of the fault patterns used by the manifest."""
import http.client
import socket
import threading
import unittest

from fixture import Fixture, Server


class FixtureTest(unittest.TestCase):
    def test_faults_and_recovery(self):
        fixture = Fixture("127.0.0.1", 0, "127.0.0.2", "127.0.0.3")
        fixture.set_fault("healthy")
        port = fixture.service.server_port
        fixture.port = port  # Reopen the same port after listener-unavailable.
        admin = Server(("127.0.0.1", 0), fixture.control_handler())
        thread = threading.Thread(target=admin.serve_forever, daemon=True)
        thread.start()

        def get(path, source):
            conn = http.client.HTTPConnection("127.0.0.1", port, timeout=2,
                                              source_address=(source, 0))
            try:
                conn.request("GET", path)
                response = conn.getresponse()
                response.read()
                return response.status
            finally:
                conn.close()

        try:
            for fault, ready_office, backend_office, ready_server in [
                ("healthy", 200, 200, 200),
                ("application-error", 503, 200, 503),
                ("backend-error", 503, 503, 503),
                ("office-access-restriction", 403, 403, 200),
            ]:
                with self.subTest(fault=fault):
                    fixture.set_fault(fault)
                    self.assertEqual(get("/ready", "127.0.0.2"), ready_office)
                    self.assertEqual(get("/backend", "127.0.0.2"), backend_office)
                    self.assertEqual(get("/ready", "127.0.0.3"), ready_server)
                    self.assertEqual(get("/ready", "127.0.0.4"), 403)
            fixture.set_fault("listener-unavailable")
            with self.assertRaises(OSError):
                socket.create_connection(("127.0.0.1", port), timeout=1)
            conn = http.client.HTTPConnection("127.0.0.1", admin.server_port, timeout=2)
            conn.request("POST", "/fault", '{"fault":"healthy"}', {"Content-Type": "application/json"})
            response = conn.getresponse()
            self.assertEqual(response.status, 200)
            response.read()
            conn.close()
            self.assertEqual(get("/ready", "127.0.0.2"), 200)
            with self.assertRaises(ValueError):
                fixture.set_fault("not-a-fault")
            self.assertEqual(fixture.state()["fault"], "healthy")
        finally:
            admin.shutdown()
            admin.server_close()
            thread.join()
            fixture.set_fault("listener-unavailable")


if __name__ == "__main__":
    unittest.main()
