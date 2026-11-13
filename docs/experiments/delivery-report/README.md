# Фактична доставка у read-only HTML

Перевірено робоче дерево після ed51a77 з новою SQL/HTML проєкцією.
Docker image: sha256:3c860d68d048330ed66b09bf0f80fc16f5fcb177ed335bfa44129ff6bd56a74c.
live.json містить real HTTPS 503→200, firing під час Agent loss, restart
Controller та stable event ID. htmlDeliveryVerified=true означає, що helper
дочекався саме event ID resolved з delivered у HTML після commit доставки.
Сирий receiver audit може містити попередні події; записи цього прогону
визначаються firing/resolution event_id. Відібрані два логічні events
receiver дедуплікував. Секрети, DSN, приватні ключі не експортувалися.

postgres-smoke.log містить реально виконаний TestPostgresWebhookSummaryIsolation:
порожня черга, rule isolation, pending/leased/delivered/failed/cancelled,
останній attempt/status та відсутність старої endpoint revision у вибірці.
Цей projection test змінює лише власні fixture rows тимчасової тестової БД;
він не замінює реальну мережеву delivery перевірку з live.json.

Один integration прогін, не гарантія exactly-once чи статистичної доступності.
Offline CLI без БД не реконструює delivery summary з assessment JSON.
Відтворення: scripts/lab_notifications.py на актуальному Compose-стенді.
