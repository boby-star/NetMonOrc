# Історія діагностики: REST, React та CLI (R12)

Підключено чинні маршрути OpenAPI:

- `GET /api/v1/diagnostics`: сесії PostgreSQL, фільтри monitor/state/since/until.
- `GET /api/v1/diagnostics/{id}`: збережений результат і використаний бюджет.
- `GET /api/v1/diagnostics/{id}/tasks`: фізичні задачі Agent та отримані Observation IDs.

Потрібна cookie-сесія або PAT зі scope `diagnostics:read`; bootstrap Assessment
reader не надає такого доступу. Для цих GET CSRF token не потрібний.
Невідомий session ID повертає 404; помилка БД — 503; read admission — 429.
Захист від невідомих/дубльованих фільтрів, некоректного часу та необмежених
вибірок спільний для клієнтів. limit — 1..200, за замовчуванням 50.

Список сесій упорядкований startedAt DESC/id DESC. Cursor містить саме DB
час із точністю PostgreSQL та прив’язаний до monitor/state/часових фільтрів.
Tasks упорядковані id ASC, cursor прив’язаний до session. Це історичний read
model без блокування виконання planner. Під час роботи сесії можуть з’являтися
нові задачі: для повного актуального переліку оновіть першу сторінку.

## Як читати результат

`budgetUsed.additionalChecks` — durable reservations фізичних probes;
втрата frame або відсутність Observation не повертає витрачений бюджет.
Task state показує збережений reserved/accepted/rejected/finished, без
вигаданого переходу в failure після deadline. `observationIds` і `evidenceIds`
містять тільки фактично отримані діагностичні спостереження. Evidence IDs
цього read model не є повним переліком початкових baseline доказів planner:
вони не зберігалися в ledger сесії й не реконструюються за поточними даними.

`remainingHypotheses` відсутнє без збереженого набору у звіті, зокрема для
перерваної Controller restart сесії. Відомий порожній набір — `[]`.
`inconclusive`/stopReason зберігаються. Це статус діагностичного висновку,
а не стан доступності Monitor.

Для нових runs зберігається profileRef. Legacy runs не отримують вигадану
назву поточного профілю. Наявний research mode `static` явно відображений
у HTTP enum разом із fixed/adaptive; він не додається до DSL і не видається
за fixed. Уточнено тип budgetUsed; нових HTTP маршрутів немає.
Check specs, target URLs, HTTP body, extraction values і credentials не
потрапляють у ці DTO. API не змінює збережені budgets, outcomes чи hypotheses.

## Панель і CLI

React показує список сесій, фільтр стану, результат та таблицю фізичних
задач із pagination і ручним refresh. Сторінка видима за diagnostics:read.
Обчислення planner не дублюються в браузері.

```bash
netmonorc get --context-file /private/context.json diagnostics
netmonorc get --context-file /private/context.json --state inconclusive diagnostics
netmonorc get --context-file /private/context.json diagnostics SESSION_ID
netmonorc get --context-file /private/context.json --tasks --limit 20 diagnostics SESSION_ID
```

Сервер підтримує monitor/time фільтри; спеціальних CLI flags для них цей
етап не додає. `--cursor` продовжує поточну вибірку; flags стоять перед kind.
У R13 підключено [ручний start/cancel](diagnostic-control.md);
історичний read model цього етапу збережено.

## Перевірка

Критичний PostgreSQL/HTTPS use case: однакові startedAt, keyset без повторів,
прив’язка cursors, legacy static/restart без вигаданих hypotheses, scope,
відсутній session, task attempts та reservations без результату, відсутність
check_spec/body у DTO. Compose перевіряє читання реальних автоматичних runs
і фізичних задач через CLI на trusted TLS. React перевірено production build;
DOM clicks не автоматизовані. Докази: [diagnostic-history-r12](../experiments/diagnostic-history-r12).
