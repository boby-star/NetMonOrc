# Історія спостережень у REST та React (R3)

У R4 додано також PAT із resources:read; [інструкція](../deployment/personal-api-tokens.md).

GET `/api/v1/monitors/{name}/observations` читає фактичні рядки PostgreSQL.
Це історія одного налаштованого Monitor, включно з історичними ревізіями та
діагностичними probes. Вона не подає дані назад до Assessment Engine.

Контракт — Observation/PageObservations з наявного OpenAPI: observationId,
monitorId, checkId, agentId, networkZone, configRevision, executionStatus,
probeOutcome, reasonCode, startedAt/finishedAt/receivedAt, durationMs, values
та збережені diagnosticSessionId/diagnosticTaskId. Відсутні failureDomain
та isReplay не вигадуються. ReceivedAt береться зі стовпця БД, не з Agent.
Збережені scalar values віддаються без перетворення int64 у float64;
таблиця React наразі показує статуси й provenance, без обчислення values.

Фільтри zone та включний діапазон since/until (RFC3339) застосовуються до
finished_at; limit 1..200, default 50. Неприпустимі, повторні або невідомі
параметри дають 400. Порожня історія — 200 з items=[] і nextCursor=null.

Порядок — finished_at DESC, id DESC. Opaque cursor містить точний ключ
останнього рядка та hash ідентичності Monitor/фільтрів. Зміна фільтра з тим
самим cursor відхиляється. Це keyset live-history, не snapshot: нові рядки
переглядаються після оновлення першої сторінки. Курсор не надає доступу;
автентифікація перевіряється для кожного запиту. Для межі сторінки береться
часовий стовпець PostgreSQL, а не nanoseconds payload, оскільки точність
PostgreSQL — microseconds. Інакше рівні часові мітки могли б повторюватися.

Лише browser session із resources:read. Legacy bootstrap bearer зберігає
доступ лише до Assessment. Запит має timeout 3s і ділить один read slot
з Assessment HTTP; перенавантаження — 429/Retry-After, збій БД — 503.
Індекс (monitor,finished_at DESC,id DESC) підтримує обмежене читання.
Результати не кешуються браузером. React підтримує зону, оновлення першої
сторінки та перехід до старіших записів; не змішує failed execution і
негативний probe та не визначає власний стан ресурсу.

Критична перевірка без повного розподіленого стенда:

```sh
python3 scripts/identity_smoke.py
```

Потрібні Go та Docker на тій самій машині. Скрипт запускає лише власну
одноразову PG із випадковим localhost-портом, створює тестову БД, запускає
один інтеграційний тест HTTPS/session/password/history, видаляє власний
контейнер і тимчасовий DSN. Він не підключається до інсталяцій користувача.
Перевіряє keyset для однакових timestamp, фільтр, відсутність дублікатів,
некоректні параметри та всі критичні зміни пароля/сесії. Кліки в браузері
не автоматизовані.
