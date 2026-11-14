# Тест webhook через durable outbox (R16)

POST `/api/v1/webhook/test` приймає рівно `{}`. Погоджене поле message
відсутнє в запиті та події. URL/target не приймаються. Потрібні admin +
alerts:manage, cookie також вимагає CSRF. Sender має бути configured/enabled.

```json
{"event_id":"UUID","event_type":"webhook.test","occurred_at":"RFC3339 timestamp","schema_version":"1"}
```

Це окрема notification event, без створення Alert або alert transition.
Event/outbox/Operation/audit комітяться разом. HTTP 202 та Location означають
queued; completed — лише HTTP 2xx. Result містить eventId, deliveryState,
attempts і nullable lastHttpStatus. Retry лишає Operation running; вичерпання
спроб/TTL або зміна endpoint завершує її failed. Статус зберігається після restart.

Використано чинні Sender, TLS/allowlist/auth secretRef/redirect policy,
lease/retry/backoff/TTL. Прямого network shortcut у handler немає. Один
pending test поточної endpoint revision допускається одночасно; disabled,
unavailable або інший pending test — 409. POST не повторюється автоматично.
Після втраченої відповіді перевірте operations/deliveries; новий POST після
завершення є новим явним тестом. RequestId не вводиться в порожній body.

## Сумісність

Queue event identity тепер належить notification ledger. Міграція зберігає
існуючі outbox IDs/payload і переносить FK; test не потребує fake Alert.
Alert transitions та history залишаються незмінними.

GET operations читає typed envelope: config.apply потребує resources:read,
webhook.test — alerts:read. Kind filter застосовується перед pagination;
ID DESC і наявні filters/cursor збережені. Config reconciliation обробляє
лише config.apply; webhook result не інтерпретується як Monitor targets.
Config UI показує config operations, webhook panel — test progress.

## UI та CLI

React має кнопку тесту й ручне оновлення статусу, attempts та HTTP status.
Delivery table також показує test event без URL/payload/secrets.

```bash
netmonorc webhook test --context-file /private/context.json --wait --timeout 45s
netmonorc get --context-file /private/context.json operations OPERATION_ID
```

Wait опитує GET і не повторює POST; потрібні alerts:manage та alerts:read.
Після timeout використайте збережений Operation ID для перевірки.

Критичний PG/HTTPS use case перевіряє upgrade старого FK без зміни черги,
точні чотири поля події, scope/CSRF, відхилення message/URL, single pending,
retry тим самим ID, progress і endpoint fencing. Compose receiver дає 503,
потім 200 для одного test ID; counts alerts/events незмінні, completed
Operation читається після restart. React build пройшов; DOM clicks не
автоматизовані. Докази: [webhook-test-r16](../experiments/webhook-test-r16).
