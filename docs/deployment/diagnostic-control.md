# Ручна діагностика й скасування (R13)

Реалізовано наявні POST `/api/v1/monitors/{name}/diagnostics` і
`/api/v1/diagnostics/{id}/cancel`. Потрібні admin + diagnostics:run;
cookie mutations також перевіряють Origin/CSRF. Actor credential повторно
перевіряється у транзакції. DTO — той самий DiagnosticSession, HTTP 202
і Location читання. Ручний запуск та auto Tick використовують один coordinator,
спільні durable incident admission, фізичні reservations, budget та cooldown.

## Запуск

Явно виберіть DiagnosticProfile з каталогу БД. Він може відрізнятися від
профілю автоматичного binding: компіляція, live limits і egress залишаються
обов’язковими. Сесія фіксує hash вибраного профілю. Це не PUT Monitor;
регулярний план і його effective revision не змінюються. Task авторизований
чинним mTLS Controller і активною enabled lease/revision Agent. У звичайному
режимі використовується fixed/adaptive вибраного профілю; дослідницький
static-comparison flag зберігає значення для цього runtime.

Новий ручний запуск відхиляється при disabled, rollout іншої desired revision,
застарілому Assessment, відсутності придатного Agent, зайнятому coordinator,
вичерпаному incident admission чи cooldown. Поточний runtime виконує один
вибраний Monitor. Public request не обходить ці обмеження через reason=trigger.

Episode не скидається reconnect, revision, cancel чи повторним запитом. Межа —
актуальний healthy з повним coverage. Ручний запуск на healthy має окремий
episode з чинним cooldown. Це раніше погоджена семантика, не нова формула.

RequestId необов’язковий у HTTP; для надійного retry його треба зберегти.
CLI вимагає явного ID. TTL — 24h у межах user ID: monitor + нормалізований
body мають збігатися, інакше 409 IDEMPOTENCY_KEY_REUSED. Default reason=manual
нормалізується до hash. Recovery повертає ту саму поточну/термінальну сесію
без нового виконання, зокрема після restart або disabled. Це не дозвіл нового
запуску при disabled. Відкликаний credential не може виконати recovery.

Worker прив’язаний до runtime context, а не HTTP request: завершення відповіді
не зупиняє діагностику. Під час Controller restart незавершений run зберігає
наявне fail-closed завершення inconclusive/controller_restart; бюджет не
перезапускається. Система не обіцяє продовження того самого physical probe.

## Скасування

Спершу БД атомарно фіксує session cancelled/operator_cancelled, unfinished
tasks cancelled та час завершення для cooldown. Бюджет не повертається.
Після COMMIT coordinator скасовує worker; DiagnosticRunner надсилає best-effort
TaskCancel через той самий stream. Agent перериває context лише відповідної
задачі; regular monitoring не скасовується. Якщо frame втрачено, діють чинні
probe deadline/lease. Старі Agent без нового frame не гарантують ранню зупинку.

Пізній receipt не зриває потік. Пізній валідний Observation залишається
історичним доказом сесії, але не змінює cancelled task/session і не потрапляє
до regular live Assessment. Завершення planner не перезаписує cancelled.
Повтор cancel — 202; completed/inconclusive — 409; missing ID — 404.

## Інтерфейси

React має форму profileRef, ручний запуск із поточним ID для retry, кнопку
«Новий запит» для іншого ID та cancel для running session. ID форми живе
в межах відкритого компонента; для recovery після перезавантаження сторінки
використайте збережений requestId через CLI/API. Після втраченої відповіді
оновіть історію й повторюйте той самий запит, а не створюйте новий ID.

```bash
netmonorc diagnostics run --context-file /private/context.json --profile web-lab --request-id incident-check-001 research-web
netmonorc diagnostics cancel --context-file /private/context.json SESSION_ID
netmonorc get --context-file /private/context.json diagnostics SESSION_ID
```

Flags стоять перед Monitor/session ID. PAT читається з приватного context.

## Перевірки та межі

Критичний real PostgreSQL/HTTPS use case перевіряє admission/replay/body reuse,
cooldown/episode, revision fencing, cancel з незмінним budget, late receipt,
late Observation та недопустимий overwrite terminal state. Agent receipt/lease
і spool use cases також перевірено. Compose запускає явний fixed profile з
12s native HTTP probe, скасовує вже accepted task і отримує cancellation
Observation раніше звичайного завершення. Recovery після process restart
повертає ту саму cancelled session; disabled відхиляє новий start.

Математика planner/assessment не змінювалася. Browser DOM clicks не
автоматизовані; React перевіряється production build. Докази:
[diagnostic-control-r13](../experiments/diagnostic-control-r13).
