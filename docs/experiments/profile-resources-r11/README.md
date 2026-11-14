# DiagnosticProfile resource integration R11

Робоче дерево після 2127468, дата 2026-11-14. Фінальний production image
ідентифіковано в image.txt. Ізольований Compose project nmo-profile-final-r11,
prefix 172.19, два справжні Agent із mTLS, PostgreSQL, Blackbox і native HTTP.

Сценарій scripts/lab_sessions.py перевіряє profile-only CLI apply --wait:
план показує залежний Monitor, Operation містить implicit target, обидва Agent
підтверджують нову ревізію, поточний Assessment повертається до healthy.
Monitor зберігає власні resourceVersion/generation. CLI читає збережений
профіль; process restart не підміняє його початковим файлом. Зворотний apply
повертає оригінальний профіль і поточний healthy. Усі попередні критичні
CLI/apply/auth/history/diagnostic-toggle/Agent reconnect сценарії також пройшли.

Окремий scripts/identity_smoke.py: disposable PostgreSQL, справжній HTTPS,
TestPostgresBrowserSessionsAndCSRF із новим profile use case. Перевірено
unordered mixed batch, однакові імена різних kind, dependent hash/version,
очікування ACK, stale If-Match, enabled delete guard, optional missing ref,
bootstrap marker і відхилення activation без профілю. Не змінено математичні
алгоритми diagnostics/assessment.

Go build/vet, тести зачеплених пакетів та TypeScript/Vite production build
пройшли. Browser DOM clicks не автоматизовані. live.json містить тільки
санітизовані результати; cookies, PAT, passwords, DSN і ключі не експортуються.
Для повторення потрібен власний новий project без користувачів, prepare/up,
потім lab_sessions.py. Дані volumes після down зберігаються.

Це готовність інтеграції DiagnosticProfile, а не всього розширеного продукту:
один active Monitor/fixed roster залишаються чинними runtime обмеженнями.
