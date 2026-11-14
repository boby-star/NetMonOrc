"""Only critical browser identity/history use cases, against a disposable local PG."""
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
IMAGE = 'postgres:17.6-alpine@sha256:ef257d85f76e48da1c64832459b59fcaba1a4dac97bf5d7450c77753542eee94'


def main():
    name = 'nmo-identity-' + secrets.token_hex(4)
    owned = False
    try:
        subprocess.run(['docker', 'run', '--rm', '-d', '--name', name,
                        '-p', '127.0.0.1::5432', '-e', 'POSTGRES_HOST_AUTH_METHOD=trust', IMAGE],
                       check=True, stdout=subprocess.DEVNULL, timeout=60)
        owned = True
        for _ in range(100):
            if subprocess.run(['docker', 'exec', name, 'pg_isready', '-U', 'postgres'],
                              stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5).returncode == 0:
                break
            time.sleep(.2)
        else:
            raise RuntimeError('PostgreSQL startup timeout')
        endpoint = subprocess.check_output(['docker', 'port', name, '5432/tcp'], text=True, timeout=5).strip()
        if not endpoint.startswith('127.0.0.1:'):
            raise RuntimeError('Test PostgreSQL must be bound only to localhost')
        port = int(endpoint.rsplit(':', 1)[-1])
        (ROOT / '.local').mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='identity-', dir=ROOT / '.local') as directory:
            dsn = Path(directory) / 'postgres.dsn'
            dsn.write_text(f'postgres://postgres@127.0.0.1:{port}/postgres?sslmode=disable')
            dsn.chmod(0o600)
            env = os.environ.copy()
            env['NETMONORC_TEST_DSN_FILE'] = str(dsn)
            subprocess.run(['go', 'test', './internal/storage', '-run',
                            '^TestPostgresBrowserSessionsAndCSRF$', '-count=1', '-v', '-timeout=60s'],
                           cwd=ROOT, env=env, check=True, timeout=90)
        print('PASS: critical HTTPS identity, users, PAT, Monitor CRUD and revision isolation', flush=True)
    finally:
        if owned:
            subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, check=True, timeout=20)


if __name__ == '__main__':
    main()
