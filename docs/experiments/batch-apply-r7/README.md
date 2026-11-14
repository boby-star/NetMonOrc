# Apply та operations R7

Робоче дерево після 274b816. critical.log: один цільовий real PG/HTTPS тест,
включно з batch rollback, відсутністю partial mutation, idempotent replay без
нового resourceVersion, conflict body reuse, pending невибраного Monitor,
superseded operation, generation fencing і completed тільки після нового ACK.

live.json: приватний Compose project nmo-apply-r7, prefix 172.23, production
image (image.txt). POST apply → persisted desired → Controller reload → mTLS
Agents/probes → новий Assessment та completed. Після process restart повтор
того самого batch повертає ту саму completed Operation, без повторного upsert.
Відновлення початкового manifest також застосоване пакетом і підтверджене.
Попередні critical users/PAT/history/revision cases проходять. Credentials
не включені в докази. Browser DOM clicks не автоматизовані.

Відтворення: scripts/identity_smoke.py; для нового disposable project —
scripts/lab.py prepare/up і scripts/lab_sessions.py з project/prefix.
Наявні identities не перезаписуються. Scope залишається одним active Monitor.
