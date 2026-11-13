# Експлуатаційні перевірки

Linux BOOTTIME lease guard відповідає реалізації cc92fc4. Smoke виконано
перед її commit із тим самим Go-кодом; документація додана після прогону.
agent-smoke.log містить реальні reconnect/replay/TTL та PostgreSQL checks.
Справжнього suspend ОС не було: розходження годинників перевірено unit-тестом.

backup-restore.json — результат pg_dump з приватним mode 0600 і pg_restore
в окрему тимчасову БД. Після перевірки ця БД видалена; робоча не змінювалась.
3103 observations, 60 diagnostic runs, alert і outbox реально відновлено.
Dump та PKI містять приватні дані й залишені лише в .local, у Git їх немає.
Це перевірка відновлення БД, не повний failover або відновлення Agent spool.

Фінальний Docker check виявив і виправив regression lifetime: спільний
exporter був прив'язаний до cancellable context одного probe. Після
виправлення він живе в process context; guard lease перевіряється перед
використанням. stable-lease.json фіксує 31 послідовний healthy/full-coverage
snapshot і нуль нових regular execution failures після виправлення.
Це охоплює кілька HTTP/TCP/DNS циклів та lease renewal; OS suspend не виконувався.
Image hash записано в JSON. Перевірено робоче дерево з виправленням після daf6b48.
