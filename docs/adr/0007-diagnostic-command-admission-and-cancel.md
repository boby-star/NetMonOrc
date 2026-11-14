# ADR 0007: Спільний admission та durable cancel

Статус: прийняте рішення реалізації дорученого public start/cancel за
погодженими episode/cooldown/enabled правилами. HTTP маршрути, математичні
моделі та DSL не змінюються.

Manual і auto ділять один runtime coordinator. Mutex захищає запуск worker,
DB transaction — incident admission, фізичні reservations і поточну desired
revision. Start requests мають per-user idempotency 24h, як config/apply;
monitor входить до identity запиту. Recovery є читанням результату вже
прийнятого запуску, а не повторним виконанням. Унікальний worker прив’язаний
до runtime context і не зупиняється через завершення HTTP response.

Явний ручний профіль компілюється з каталогу. Його hash перевіряється з БД
у transaction і зберігається для сесії. Ефемерний diagnostic binding worker
не змінює regular Monitor manifest/revision; Agent task залишається під чинною
Controller-owned enabled lease. Egress та типізовані probe validators чинні.

Скасування спочатку commit-ить terminal cancelled і час завершення в БД,
потім скасовує worker context і надсилає additive TaskCancel frame. Це не
розподілена транзакція з Agent: lost frame обмежений deadline/lease. Ledger
не повертає reservation. Late receipts/observations дозволені як історичні,
але не скасовують terminal state і не перезаписують live regular evidence.

Публічний POST не вважається завершеним physical probe. HTTP 202 означає
прийняту сесію або durable cancel. Instance restart, reconnect і конфігураційні
ревізії не створюють автоматичного повторного фізичного запуску.
