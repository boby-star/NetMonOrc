# ADR 0003: моніторне правило й durable HTTPS-доставка

Статус: прийнято для мінімального прототипу.

Підстава: мінімальний план передбачає один webhook; DSL §§13–14 уже задає
AlertRule, WebhookEndpoint та payload. Користувач погодив окремий cooldown
для firing/resolved, reset pending при unknown/error/gap/revision і
переозброєння тільки через when=false→true. Нові зовнішні контракти не потрібні.

Використовуємо чистий CEL/FSM evaluator і PostgreSQL transaction outbox
всередині Controller. Один worker резервує attempt/lease перед запитом;
стабільний event_id дає одержувачу дедуплікацію. Обмежені retry/backoff
не вимагають Redis/Kafka або окремого сервісу.

Межі: один explicit monitorRef, monitor scope, IP-literal HTTPS,
попередньо встановлений bearer secret. Частота tick 1s; gap>3s/restart
консервативно скидає pending. Черга має локальний TTL 24h. Terminal rows
зберігаються, retention не реалізовано. Відправник не гарантує exactly-once.
Непідтримувані DSL можливості відхиляються явно.

Приватний HTTPS receiver із fault control та SQLite — лише fixture стенда.
HTML-звіт показує стан алерту й enqueue, не стверджує доставку.
Докладно: [alert-delivery.md](../architecture/alert-delivery.md).
