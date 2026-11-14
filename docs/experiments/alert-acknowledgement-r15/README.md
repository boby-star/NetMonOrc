# Alert acknowledgement R15

Робоче дерево після f412e57, дата 2026-11-14. Production image в image.txt.
Compose project nmo-alert-ack-r15, private prefix 172.19, два mTLS Agent,
PostgreSQL. Без host ports.

scripts/lab_sessions.py підтверджує реальний firing через CLI з серверним
ETag і note stdin. Down Assessment та outbox count не змінюються. Після
Controller restart acknowledgedAt зберігається. Наступний реальний firing
очищає поточний receipt; старий tag отримує 412. API перевіряє cookie CSRF,
required If-Match та scope. Попередні критичні workflows збережені.

scripts/identity_smoke.py перевіряє per-cycle ledger, timestamp-preserving
repeat, conflict іншої note, Unicode boundary/NUL, inactive conflict,
незмінність FSM/Assessment/events/outbox, збереження receipt при unknown
і reset при новому firing. Trusted-TLS API client test пройшов після
додавання транспорту opaque ETag. Go build/vet і React build пройшли;
DOM clicks не автоматизовані.

Live JSON санітизовано; notes/credentials/keys/DSN не експортуються.
Для повторення потрібен власний новий disposable project без users:
lab.py prepare/up, потім lab_sessions.py з відповідними project/prefix.
Named data volumes зберігаються після down. Rule/webhook CRUD/test і
решта розширеного scope цим етапом не оголошуються готовими.
