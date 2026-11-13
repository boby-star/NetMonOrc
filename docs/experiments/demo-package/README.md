# Приватний пакет і офлайн-відтворення звіту

Перевірено робоче дерево після a0d9494 з delivery snapshot і package helper.
Образ та hash фактичного архіву зазначені в verification.json.
Реальні HTTPS retry/restart перевірено lab_notifications.py перед створенням
архіву; HTML дочекався committed delivered конкретної resolved події.

Package містив лише README.txt, SHA256SUMS та п'ять HTML/JSON snapshots.
Перевірено права 0600, всі hashes, відсутність runtime secret markers,
створення HTML офлайн через netmonorc-report і відмову для чужого Monitor.
Unit-тести перевіряють allowlist, відсутність обов'язкового report та
незмінність попереднього архіву при повторному output path.

Сам приватний архів залишено в .local, у Git його не додано. Verification
не є backup БД або доказом єдиного атомарного snapshot усіх файлів.
Контейнери зупинені, named volumes збережені.

Відтворення: [demo-package.md](../../deployment/demo-package.md).
