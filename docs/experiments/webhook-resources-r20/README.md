# Persisted WebhookEndpoint integration R20

Дата: 2026-11-15 (Europe/Kiev). Реалізація: 9c4d3ad; сценарій: 92b79df; CLI recovery: e32928a.
Project: nmo-webhook-resources-r20-final; private prefix 172.19,
PostgreSQL та два mTLS Agent. Host ports не публікуються.

Live scenario через CLI читає startup endpoint із PostgreSQL, створює inactive
replacement і змінює timeout активного каналу в одному apply. Очікує
webhook_configuration_loaded для обох targets, перезапускає Controller,
відновлює private CLI context через stdin і перевіряє збережений timeout.
Реальна test delivery проходить persisted Sender до HTTPS receiver з HTTP 200.
Потім неактивну заміну видалено через ETag/CSRF API, початковий канал відновлено,
а нові Agent heartbeat/revision proofs підтверджені після reload.

Збережені попередні critical use cases: auth, resources/apply, reconnect,
manual diagnostics, alert lifecycle/acknowledgement, outbox retry,
managed AlertRule, dashboard та observation detail. Export JSON містить
лише результати перевірок без credentials, cookie, DSN та приватних ключів.

Disposable PostgreSQL/HTTPS smoke перевірив single-active invariant,
inactive replacement, outside-CIDR rejection, missing-secret atomic rollback,
If-Match, stale sender/test fencing, runtime readiness, delete та відсутність
bootstrap resurrection. Go build/vet, критичні HTTPS sender tests і React
production build пройшли. DOM clicks не автоматизовані.

Для повторення потрібен новий власний project без users: lab.py prepare,
lab.py up, lab_sessions.py --output LOCAL_PRIVATE_FILE. Після lab.py down
named volumes зберігаються. Повний відкритий scope — IMPLEMENTATION_STATUS.md.

Critical CLI test додатково моделює COMMIT + втрату HTTPS-відповіді:
повтор має той самий requestId/body та повертає ту саму operation.
TLS trust failure і 412 не повторюються як transient errors.
