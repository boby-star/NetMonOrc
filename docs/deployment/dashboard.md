# Огляд моніторингу (R17)

Підключено GET `/api/v1/dashboard/summary`, React overview та CLI get dashboard.
Це read projection наявних джерел; frontend не оцінює доступність самостійно.

Склад відповіді визначають scopes: resources:read — monitorStates/coverageStates,
agents:read — agentCounts, alerts:read — firingAlerts, diagnostics:read — останні
5 діагностичних сесій. Неавторизовані поля відсутні. Bootstrap Assessment token
не відкриває summary. Query parameters не підтримуються. Помилка джерела — 503,
read admission — 429; порожній дозволений компонент має нульові counters/масив.

Кількість desired Monitor і Assessment активної effective revision читаються
в одному SQL snapshot. Freshness/revision checks та coverage projection спільні
з детальним Assessment API: старий healthy не потрапляє в summary після rollout
або втрати актуальності. Невибрані Monitor мають unknown. Лише один active
Monitor виконується поточним runtime; це не приховується агрегованим лічильником.

Agent counts походять із live fixed-roster sessions, окремо від health ресурсу.
Online Agent не робить Monitor healthy. Firing alerts — збережений lifecycle,
acknowledgement його не зменшує. Recent diagnostics показують фактичні statuses,
включно з inconclusive/cancelled. observedAt — час складання. Різні компоненти
читаються послідовно, не оголошуються глобально атомарним snapshot live streams.

```bash
netmonorc get --context-file /private/context.json dashboard
```

React використовує ручний refresh і показує лише отримані поля. API не замінює
деталі Assessment, історії, Agents або Diagnostics.

Критичний PG/HTTPS сценарій перевіряє unselected/stale/revision mismatch →
unknown, незалежність Agent online та scope isolation. Compose перевіряє
healthy/sufficient/online counters і реальні recent sessions через CLI/API.
Go build/vet та React build пройшли; DOM clicks не автоматизовані. Докази:
[dashboard-r17](../experiments/dashboard-r17).
