# Durable webhook test R16

Дата: 2026-11-15 (Asia/Jerusalem), робоче дерево після 5a45916.
Production image в image.txt. Project nmo-webhook-test-final-r16,
private prefix 172.19, PostgreSQL і два mTLS Agent, без host ports.

Compose запускає CLI test --wait. Receiver дає 503 і потім 200; requests
мають один ID. Body містить рівно event_id, event_type=webhook.test,
occurred_at і schema_version=1. Alert/event counts незмінні; completed
Operation читається після restart. Попередні critical workflows збережені.

Real PG/HTTPS перевіряє migration старого FK без зміни IDs/payload,
scope/CSRF, no message/URL override, single pending, retry/progress,
endpoint fencing і operation authorization до pagination. Go build/vet,
критичні тести та React build пройшли. DOM clicks не автоматизовані.
Для повторення потрібен новий власний project без users: prepare/up,
потім lab_sessions.py. Credentials/keys/payload не експортуються.

Перший stand не завершив startup через повний диск. Видалено лише unused
Docker build cache; source, images і data volumes збережено. Артефакт live.json
належить повторному прогону на новому stand після відновлення вільного місця.
