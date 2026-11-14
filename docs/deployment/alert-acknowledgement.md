# Підтвердження алертів (R15)

Підключено POST `/api/v1/alerts/{id}/acknowledge` до Controller, React і CLI.
Потрібні admin + alerts:manage; cookie mutation перевіряє Origin/CSRF.
Actor credential повторно перевіряється у DB transaction під час запису.
Підтвердити можна лише firing. Це запис отримання алерта оператором:
FSM, Monitor Assessment, suppression, transition history і outbox не змінюються.

## Захист від застарілої вкладки

GET `/api/v1/alerts/{id}` повертає strong ETag усієї representation Alert.
POST acknowledge вимагає цей `If-Match`: без header — 428, стара версія — 412.
Неактивний алерт чи неузгоджена projection — 409. Перевірка і запис відбуваються
під тим самим row lock, що й EvaluateAlert. Вкладка, відкрита до нового firing,
не може підтвердити новий цикл старою версією.

ETag — opaque значення, а не DSL field або resourceVersion Monitor. Header
додано в межах погодженого правила конкурентних змін Q07; HTTP контракт
уточнено без нового маршруту. UI при 412 просить оновити деталі, не повторює
POST автоматично й не замінює очікувану версію непомітно.

## Durable receipt та примітка

Перший receipt циклу фіксує час, actor ID/name і необов’язкову note в окремій
таблиці. Note: до 1024 Unicode characters, без NUL. Вона не потрапляє до
webhook, логів або публічного Alert DTO. Public metadata — acknowledgedAt.
Audit записує operator action, без вмісту note.

Повтор із поточним ETag і тією самою note повертає початковий receipt та час
без дубліката. Інша note для вже підтвердженого циклу — 409. Після втраченої
відповіді GET дозволяє перевірити acknowledgedAt та отримати поточний tag;
повтор зі старим tag отримує 412 за загальним HTTP precondition правилом.

AcknowledgedAt зберігається при наступному unknown, під час firing і після
resolved, але скидається при відкритті нового циклу pending/firing. Ledger
попередніх циклів залишається. Повторне спрацювання потребує нового receipt.
Підтвердження не вимикає правило, не приглушує майбутні notification events,
не означає healthy і не повертає бюджети діагностики.

## Інтерфейси

React показує час підтвердження; admin з alerts:manage бачить форму note
для непідтвердженого firing. Viewer читає acknowledgedAt, але не підтверджує.
Форма використовує tag відкритих деталей. Після успіху кнопка приховується.

```bash
netmonorc alerts acknowledge --context-file /private/context.json ALERT_ID
netmonorc alerts acknowledge --context-file /private/context.json --note-file review.txt ALERT_ID
netmonorc alerts acknowledge --context-file /private/context.json --note-file - --if-match '"OPAQUE_TAG"' ALERT_ID
```

Без explicit `--if-match` CLI спочатку GET-ить поточний Alert та його серверний
ETag; POST не має автоматичного retry. Явний tag дозволяє захистити раніше
переглянутий snapshot. Note читається з файла/stdin, не з CLI-аргументу.
Для автоматичного GET потрібний alerts:read; для POST — alerts:manage.
Shared API client зберігає opaque header; клієнт не обчислює версію сам.

## Перевірка й межі

Критичний real PG/HTTPS use case перевіряє scope/CSRF, 428/412, Unicode/NUL,
перший receipt/replay/conflicting note, незмінність FSM/Assessment/outbox,
unknown/resolve/new firing та незастосовність старого ETag до нового циклу.
Окремий trusted-TLS client test перевіряє рефакторинг транспорту.

Compose запускає реальний firing, підтверджує через CLI, перевіряє down і
незмінний outbox, перезапускає Controller та читає той самий acknowledgedAt.
Наступний реальний firing очищає receipt; старий tag відхиляється. React
перевірено production build; browser DOM clicks не автоматизовані. Докази:
[alert-acknowledgement-r15](../experiments/alert-acknowledgement-r15).

Залишаються повний AlertRule/UID lifecycle, CRUD правил/webhook, webhook test,
multi-Monitor та placement. Цей етап завершує acknowledgement поточного runtime.
