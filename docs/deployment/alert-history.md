# Перегляд алертів і доставки webhook (R14)

Підключено наявні маршрути:

- GET `/api/v1/alerts` — поточні logical alert projections.
- GET `/api/v1/alerts/{id}` — стан, timestamps, причини й докази поточного циклу.
- GET `/api/v1/alerts/{id}/history` — незмінні переходи FSM та cooldown suppression.
- GET `/api/v1/webhook/deliveries` — delivery summaries outbox поточної endpoint revision.

Cookie-сесія або PAT повинні мати alerts:read. Bootstrap Assessment token не
надає такого доступу. Помилка БД — 503, read admission — 429, missing alert —
404. Невідомі/повторні filters і некоректний limit повертають 400.

## Проєкція з реального оцінювання

Alert UUID створюється один раз під наявним row lock, разом із state/event/outbox.
Нова проєкція не обчислює CEL і не змінює Alert FSM. Вона публікує факти,
отримані під час справжнього EvaluateAlert. StartedAt позначає початок відомого
поточного циклу; firedAt/resolvedAt встановлюються фактичними transitions.
Новий цикл скидає попередні cycle timestamps. UUID logical alert залишається
сталим, а event ID окремий для кожного переходу.

Firing зберігає причини й докази спрацювання до фактичного переходу. Новий
unknown не підмінює їх; застарілий Assessment не переписує projection.
Стан resolved має докази розв’язання. Дані старих rows не реконструюються за
сьогоднішнім Assessment: проєкція з’являється після оцінювання з валідними
metadata; невідомі historical timestamps лишаються відсутніми/null. Legacy
firing без збереженої причини має порожні reason/evidence, а не вигадані докази.

Поточний runtime зберігає наявний name-based logical key rule/Monitor/scope.
Повний UID-based lifecycle AlertRule/Monitor та zone-scoped alerts ще потребують
окремого розширення registry й міграції. Цей етап не оголошує їх реалізованими.
Acknowledgement, AlertRule CRUD і webhook test ще не підключені; acknowledgedAt
поки null. Ці read routes не імітують успішних адміністративних операцій.

## Pagination та історія

Alerts підтримують monitor/state/severity/zone, limit 1..200 (default 50),
порядок id ASC. Cursor прив’язаний до filters. У поточному monitor-scoped
runtime zone=null; nonempty zone filter не вигадує зональні alerts.

History повертає реальні transition from/to/at/configRevision/suppressed,
у порядку DB createdAt DESC/id DESC. Cursor використовує DB timestamp із
точністю PostgreSQL та прив’язаний до alert ID. Suppressed transitions лишаються
в історії. suppressed=false не означає обов’язкову доставку: notifyEvents,
enabled і правила черги застосовуються окремо. Історія доступна між циклами.

## Доставка

Один delivery row відповідає outbox event для поточної endpoint revision,
а не окремій фізичній HTTP-спробі. Attempts — кількість зарезервованих спроб;
lastHttpStatus — останній збережений HTTP status (null за відсутності відповіді),
lastAttemptAt — початок останньої спроби, включно з незавершеною. Leased не
означає delivered. Status попередньої завершеної спроби може ще відображатися,
доки нова виконується.

Доступні state filter, limit/cursor за event ID ASC. Cursor прив’язаний також
до server-owned endpoint revision; старий destination не змішується з новим.
Без налаштованого endpoint список порожній. URL, auth, raw payload/response,
secretRef і lease tokens не повертаються. Retry/backoff/outbox logic не змінені;
помилка webhook не змінює стан алерта або Monitor Assessment.

## Інтерфейси та перевірки

React має список алертів із state filter, деталі, причини/evidence, історію
переходів та окрему delivery table. Оновлення ручне, pagination серверна.

```bash
netmonorc get --context-file /private/context.json alerts
netmonorc get --context-file /private/context.json alerts ALERT_ID
netmonorc get --context-file /private/context.json --history --limit 20 alerts ALERT_ID
netmonorc get --context-file /private/context.json --state failed deliveries
```

Flags перед positional arguments. Спеціальних CLI flags для всіх filters
ще немає; REST і React використовують той самий серверний read model.

Критичний PG/HTTPS use case перевіряє trigger evidence, stale/unknown data,
cycle timestamps, suppressed history, cursor bindings, scopes, legacy rows,
delivery attempts/privacy та endpoint isolation. Compose перевіряє реальні
firing/resolved transitions і delivered webhook через CLI. Go build/vet,
критичні тести та React production build пройшли. DOM clicks не автоматизовані.
Докази: [alert-history-r14](../experiments/alert-history-r14).
