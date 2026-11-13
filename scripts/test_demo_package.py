import hashlib
from pathlib import Path
import stat
import tarfile
import tempfile
import unittest
from lab import Lab


class FixtureLab(Lab):
    def __init__(self, snapshots):
        self.snapshots=snapshots
        self.requested=[]
    def require(self):pass
    def snapshot(self,name):
        self.requested.append(name)
        return self.snapshots.get(name)


class PackageTests(unittest.TestCase):
    def test_private_allowlisted_archive_and_checksums(self):
        lab=FixtureLab({'assessment.json':b'{}','report.html':b'<html></html>',
                        'delivery.json':b'{}','database.dsn':b'MUST NOT EXPORT','ca.key':b'MUST NOT EXPORT'})
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'demo.tar.gz'
            lab.package(path)
            self.assertEqual(stat.S_IMODE(path.stat().st_mode),0o600)
            with tarfile.open(path) as archive:
                self.assertEqual(set(archive.getnames()),{'README.txt','SHA256SUMS','assessment.json','report.html','delivery.json'})
                for item in archive.getmembers():self.assertEqual(item.mode,0o600)
                for line in archive.extractfile('SHA256SUMS').read().decode().splitlines():
                    digest,name=line.split('  ',1)
                    self.assertEqual(hashlib.sha256(archive.extractfile(name).read()).hexdigest(),digest)
            before=path.read_bytes()
            with self.assertRaises(FileExistsError):lab.package(path)
            self.assertEqual(path.read_bytes(),before)
            self.assertNotIn('database.dsn',lab.requested)

    def test_missing_required_report_fails_without_output(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'demo.tar.gz'
            with self.assertRaises(RuntimeError):FixtureLab({'assessment.json':b'{}'}).package(path)
            self.assertFalse(path.exists())


if __name__=='__main__':unittest.main()
