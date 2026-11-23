# Керування AlertRule (R19)

AlertRule підключений до існуючих `/api/v1/resources/{kind}`,
`/api/v1/resources/{kind}/{name}`, `/revisions`, `/status` і config
validate/plan/apply. Контракт DSL не змінено: використовуються наявні
monitorRef, channelRef, when/resolveWhen, for/cooldown, severity,
notifyEvents, enabled та description.

Актуальне доповнення: [zone scope, selectors і повний CEL context](zone-alerts.md).

Каталог перевіряє весь запропонований граф до commit. Явний monitorRef повинен існувати,
а channelRef — відповідати наявному WebhookEndpoint. Невідомі поля,
некоректний CEL, scope поза monitor/zone та відсутні посилання відхиляються. Підтримано
monitor/zone scope та selector; у R20 endpoint керується також через каталог ресурсів. Для активації
каналу залишаються потрібні парні startup flags --alert-rule та --webhook-config.
Файл AlertRule імпортується один раз; збережені зміни не перезаписуються ним
після restart, а видалений ресурс не відновлюється з bootstrap.

## Операції

Читання потребує resources:read. Запис — admin + resources:write; cookie-запити
додатково потребують CSRF. Заміна та видалення використовують ETag/If-Match,
428 без обов'язкового заголовка і 412 для застарілої версії. Generation,
resourceVersion, immutable history та audit використовують спільну реалізацію
каталогу ресурсів. Batch apply підтримує змішані Monitor/DiagnosticProfile/AlertRule
пакети, expectedVersions та чинну 24-годинну idempotency.

```sh
netmonorc get --context-file /private/context.json AlertRule
netmonorc get --context-file /private/context.json AlertRule service-outage
netmonorc plan --context-file /private/context.json -f rules.json
netmonorc apply --context-file /private/context.json -f rules.json --wait
```

У React: «Конфігурація моніторингу» → тип AlertRule → каталог/редактор.
Валідація та правила станів виконуються сервером. Batch Apply показує
очікування фактичного оцінювання правила. Status `stored` означає перевірений
запис каталогу; він сам по собі не доводить активацію правила.

## Runtime

На кожному циклі Controller читає правила вибраного Monitor. Кілька правил
оцінюються окремо в порядку name; скомпільований CEL повторно використовується
до зміни semantic hash або UID. Оновлення правила не змінює probe plan і не
перепідключає Agents. Результати правил потрапляють до того самого alert FSM,
історії та durable webhook outbox.

Перед зміною FSM транзакція бере спільний catalog lock та перевіряє UID/hash
правила й effective revision Monitor. Заміна/видалення ресурсу не дозволяє
попередньому evaluator створювати нові події після commit зміни. Catalog writers
беруть той самий lock ексклюзивно; порядок захищає від взаємного блокування
під час mixed apply. Застаріле оцінювання пропускається до наступного циклу.

Apply target AlertRule стає ready лише після збереженого оцінювання відповідного
hash на поточній ревізії Monitor, виконаного після початку операції. Зміна
правила до завершення операції дає CONFIG_SUPERSEDED. Правила невибраного
Monitor зберігаються, але очікують його виконання Controller.

Видалення pending/firing правила відхиляється з 409: видалення не є прихованим
resolve. Спочатку правило повинно вийти зі стану pending/firing; для firing потрібен
явний resolve за умовою правила. Monitor із
посиланнями AlertRule також захищений від видалення. Вимкнення правила не
вигадує подію відновлення сервісу. Математика FSM, cooldown та false→true
rearm не змінені.

API/React Alerts показують усі правила. Старі локальні alert/delivery snapshot
та HTML report показують перше правило за name; вони не є повним каталогом.
Повний UID-based lifecycle історичних логічних ключів залишається окремим
незавершеним пунктом v1alpha1; цей етап не оголошує його реалізованим.

## Перевірки

`python3 scripts/identity_smoke.py` — реальні PostgreSQL і HTTPS для CRUD,
If-Match, reference validation, старого/deleted evaluator, pending/firing delete
protection, resolved та apply readiness. Go build/vet і React production build
перевіряють інтеграцію. `scripts/lab_sessions.py` перевіряє кілька правил,
CLI apply --wait, persistence після restart та повернення початкової конфігурації.
DOM interaction цим сценарієм не перевіряється.


R20: [керування endpoint та runtime reload](webhook-resources.md). Посилання
на неактивний endpoint дозволене; доставка через нього вимкнена.
