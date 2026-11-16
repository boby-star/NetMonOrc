import json
import io
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch
from lab import Lab

class LocalUI(unittest.TestCase):
    def setUp(self):
        self.directory=tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.lab=Lab('nmo-ui-test','172.19');self.lab.work=Path(self.directory.name)
        (self.lab.work/'prepared.json').write_text(json.dumps({'project':self.lab.project,'prefix':self.lab.prefix}))
    def test_publish_only_loopback_and_persist(self):
        with patch.object(self.lab,'compose',return_value=b''):
            self.lab.configure_ui(18443)
            self.assertEqual(self.lab.ui_port(),18443)
            value=json.loads((self.lab.work/'ui.compose.json').read_text())
            self.assertEqual(value['services']['controller']['ports'][0]['host_ip'],'127.0.0.1')
            self.assertEqual(set(value['services']),{'controller'})
            self.lab.configure_ui(0)
            self.assertIsNone(self.lab.ui_port())
    def test_reject_public_or_extra_service(self):
        for mutation in ['public','database']:
            value=Lab.ui_override(18443)
            if mutation=='public':value['services']['controller']['ports'][0]['host_ip']='0.0.0.0'
            else:value['services']['postgres']={'ports':['5432:5432']}
            (self.lab.work/'ui.compose.json').write_text(json.dumps(value))
            with self.assertRaises(ValueError):self.lab.ui_port()
    def test_do_not_change_running_binding(self):
        with patch.object(self.lab,'compose',return_value=b'running'):
            with self.assertRaises(RuntimeError):self.lab.configure_ui(18443)
        self.assertFalse((self.lab.work/'ui.compose.json').exists())
    def test_compose_never_consumes_caller_stdin_implicitly(self):
        with patch('lab.subprocess.run') as run:
            self.lab.compose('ps', '--quiet')
            self.assertEqual(run.call_args.kwargs['stdin'],subprocess.DEVNULL)
            self.lab.compose('exec','-T','controller',input=b'private-test-input')
            self.assertIsNone(run.call_args.kwargs['stdin'])
            self.assertEqual(run.call_args.kwargs['input'],b'private-test-input')
    def test_bootstrap_password_pipe_is_not_an_argument(self):
        password=b'fixture-password-1234'
        stream=io.TextIOWrapper(io.BytesIO(password+b'\r\n'))
        with patch.object(self.lab,'sql',return_value='0'),patch.object(self.lab,'compose') as compose,patch('sys.stdin',stream):
            self.assertTrue(self.lab.admin('admin',True))
            self.assertEqual(compose.call_args.kwargs['input'],password)
            self.assertNotIn(password.decode(),compose.call_args.args)
    def test_multiline_password_rejected(self):
        stream=io.TextIOWrapper(io.BytesIO(b'fixture-password-1234\nsecond-line'))
        with patch.object(self.lab,'sql',return_value='0'),patch.object(self.lab,'compose') as compose,patch('sys.stdin',stream):
            with self.assertRaises(ValueError):self.lab.admin('admin',True)
            compose.assert_not_called()
    def test_existing_admin_never_prompts_or_replaces(self):
        with patch.object(self.lab,'sql',return_value='1'),patch.object(self.lab,'compose') as compose:
            self.assertFalse(self.lab.admin())
            compose.assert_not_called()

if __name__=='__main__':unittest.main()
