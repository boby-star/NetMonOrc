# Alert and delivery read integration R14

Фінальне робоче дерево після df0e5cc, дата 2026-11-14. Production Controller image
записаний у image.txt. Compose project nmo-alert-read-final-r14, private prefix 172.19,
PostgreSQL і два справжні mTLS Agent. Host ports не публікуються.

scripts/lab_sessions.py читає через CLI/API реальний logical Alert, його
firing/resolved transitions і timestamps, а також delivered webhook із
HTTP 200, attempts і lastAttemptAt. Outbox count відповідає read model.
PAT без alerts:read відхиляється; response не містить URL/payload/lease/secret.
Попередні критичні auth/config/diagnostics/cancel/reconnect scenarios збережені.

scripts/identity_smoke.py перевіряє projection transaction, trigger evidence
при unknown, збереження resolved evidence до нового циклу,
захист від stale assessment, cycle timestamps, cooldown-suppressed
history, bounded pagination/cursor bindings, endpoint revision isolation,
nullable status/attempt time та privacy. Legacy firing не отримує вигаданих
onset/evidence. Delivery failure не скасовує firing. Alert FSM не змінювалася.

Go build/vet, зачеплені unit tests і React production build пройшли.
Browser DOM interactions не автоматизовані. Live JSON санітизований;
credentials/DSN/keys не експортуються. Для повторення потрібен новий власний
project без users: lab.py prepare/up і lab_sessions.py. Named data volumes
зберігаються після down. Acknowledge і webhook test не оголошуються готовими.
