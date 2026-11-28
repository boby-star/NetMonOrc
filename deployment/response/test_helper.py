import tempfile
import time
import unittest
from pathlib import Path
from unittest.mock import patch
from helper import Policy

class PolicyTest(unittest.TestCase):
    def test_allowlist_durable_replay_and_expiry(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = str(Path(tmp) / 'journal.json')
            policy = Policy([18088], ['188.137.254.31/32'], path)
            request = {'id': 'a' * 32, 'port': 18088, 'sourceCIDR': '188.137.254.31/32', 'untilUnixMilli': int(time.time() * 1000) + 60000}
            with patch('helper.nft') as nft:
                self.assertEqual(policy.block(request)['status'], 'applied')
                # Restart + replay must not extend kernel timeout.
                restarted = Policy([18088], ['188.137.254.31/32'], path)
                self.assertEqual(restarted.block(request)['status'], 'applied')
                self.assertEqual(nft.call_count, 1)
                for change in [{'port': 22}, {'sourceCIDR': '0.0.0.0/0'}, {'sourceCIDR': '188.137.224.35/32'}, {'untilUnixMilli': request['untilUnixMilli'] + 1}, {'id': 'b' * 32, 'untilUnixMilli': int(time.time() * 1000) + 600000}, {'id': 'c' * 32, 'command': 'nft flush ruleset'}]:
                    with self.assertRaises(ValueError):
                        restarted.block(request | change)
                self.assertEqual(nft.call_count, 1)
                with patch('helper.time.time', return_value=request['untilUnixMilli'] / 1000 + 1):
                    self.assertEqual(restarted.block(request)['status'], 'rejected')
                self.assertEqual(nft.call_count, 1)
            self.assertEqual(Path(path).stat().st_mode & 0o777, 0o600)

if __name__ == '__main__':
    unittest.main()
