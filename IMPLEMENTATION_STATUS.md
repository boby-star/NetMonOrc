# Персональні токени й CLI: R4

Реалізовані POST/GET/DELETE /auth/tokens і bearer /auth/me. PostgreSQL
зберігає тільки token hash, expiry/revoked_at та scopes. Видача через browser
cookie+CSRF; owner isolation, перетин із поточною роллю та durable revocation.
PAT з resources:read читає Assessment/history; bootstrap reader лишається
обмеженим Assessment. React дозволяє видати, одноразово показати secret,
переглянути сторінки metadata та відкликати токен.
CLI login/whoami/local logout перевіряє TLS, забороняє redirect, читає PAT
через прихований prompt/stdin, зберігає приватний атомарний контекст 0600.
Перевірено критичні HTTPS/PG token use cases та CLI trust/private context;
Go build/vet і React production build проходять. Окремий Compose-сценарій
підтвердив реальні CLI login/whoami/logout та PAT після restart/revoke:
[докази](docs/experiments/personal-api-tokens-r4). Інструкція:
[personal-api-tokens.md](docs/deployment/personal-api-tokens.md).
Повні users/resource CRUD, загальні CLI commands, placement/replicas та
ResourceGroup ще не завершені. Наступні записи — історія попередніх етапів.

# Історія перевірок у панелі: R3

Додано GET `/api/v1/monitors/{name}/observations` і React-таблицю.
Дані читаються з PostgreSQL з networkZone, agent/check, execution/probe,
часом приймання, ревізією та diagnostic provenance. Keyset pagination,
limit ≤200, zone/since/until, bounded query та shared reader admission.
Поточний Assessment не змінюється через перегляд історії.
Окремий Compose-стенд із production образом перевірив login → history →
password change → restart → logout: docs/experiments/rest-history-r3.
Критичний HTTPS/PG юз-кейс охоплює cursor на однаковому timestamp,
фільтри/валідацію, login/password/session lifecycle. Деталі й відтворення:
[observation-history.md](docs/architecture/observation-history.md).

# Зміна власного пароля: R2b

API `/api/v1/auth/change-password` та React-форма інтегровані з PostgreSQL.
Успіх атомарно відкликає всі старі cookie-сесії та видає нову сесію/CSRF;
поточний пароль, чинна сесія, origin та CSRF обов'язкові. Хешування ділить
обмежений слот і rate budget із login. Неправильні currentPassword не
обходять persisted lockout. Інструкція: docs/deployment/browser-sessions.md.
Перевірено один критичний real PostgreSQL/HTTPS сценарій: відмова без CSRF,
неправильний поточний/слабкий новий пароль, відсутність змін після відмови,
відкликання двох старих сесій, нові cookie/CSRF, збереження після нового Service,
старий пароль відхиляється, новий приймається. Production React build та
цільові Go tests/vet проходять. Browser DOM automation не запускалась.
PAT, users CRUD та решта розширеного обсягу залишаються відкритими;
100% готовності всього розширеного проєкту не заявляється.

# Сесійний вхід: R2a

Реалізовано login/me/logout, PostgreSQL sessions (тільки token hash),
Argon2id, origin/CSRF, 8h expiry, 5 failures/15m lockout, bounded login
та identity audit. React має парольний вхід, session restore й logout.
Локальний netmonorc-admin створює лише першого admin без password argv.
Реальні юз-кейси: docs/experiments/browser-sessions. PAT/users CRUD та
повний RBAC решти endpoints ще не реалізовані.

# Розширення обсягу за запискою: REST API та React

Актуальний технічний план: [thesis-scope-2026-11-13.md](docs/specs/thesis-scope-2026-11-13.md).
Попередні 96% нижче описують історичний мінімальний профіль, а не новий обсяг.
R1 реалізує один read-only GET Assessment і React-перегляд. Повні auth/RBAC,
CRUD, policy refs, regular replicas/placement і групи ще не завершені.
Деталі: [rest-react-r1.md](docs/architecture/rest-react-r1.md).

# Фактичний стан мінімального прототипу

Це послідовність завершених етапів. Актуальні межі зведено наприкінці;
орієнтовна готовність мінімуму — [близько 96%](docs/planning/prototype-readiness.md).

## Перший локальний етап — реалізовано

Перший локальний наскрізний зріз: суворе декодування підмножини YAML,
перевірка параметрів, defaults, нормалізований план, SHA256 effective revision,
реальні обмежені HTTP-запити, NDJSON-спостереження та зональні оцінки.
Ваги, required, прострочені/старі результати, часова неузгодженість,
нестача доказів і технічні помилки обробляються окремо.

Пакети: `internal/config`, `internal/monitor`, `internal/probe`.
Локальний CLI: `cmd/netmonorc`. Приклад і наскрізна демонстрація:
`examples/http.yaml`, `scripts/smoke.py`.

Інтегровано реальний Blackbox Exporter 0.27.0 для HTTP. За runner
`auto`/`blackbox` генерується закрита конфігурація exporter, запускається
керований локальний процес і його метрики перетворюються на Observation.
Різні ревізії враховують вибраний runner. IP allowlist перевіряється перед
викликом exporter; redirects заборонені, TLS verification обов'язковий.
Запуск/зупинка процесу належать CLI, executable вибирає адміністратор,
не YAML. Мережевий Agent додано наступним етапом нижче.

Перевірено: `go build`, `go test ./...` (три пакети з тестами),
`go vet ./...`; smoke з реальними HTTP 200/503 і відсутністю джерела.
Критичні перевірки включають required failure попри високу H,
TTL/revision/maxSkew, дублікати, новішу execution failure,
обмеження відповіді, redirect і заборону metadata-адреси.
Додатково перевірено реальний Blackbox HTTP 200/503, надмірну відповідь,
заборону redirects, malformed metrics і HTTP-помилку самого exporter.

## Розподілений зріз — реалізовано

Окремі `netmonorc-controller` та `netmonorc-agent`, Protobuf/gRPC Connect
із mTLS 1.3 і наперед підготовленими сертифікатами, фіксований roster,
assignment checksum/revision/lease, регулярне послідовне виконання,
локальна durable черга на 1000 записів, reconnect і ACK після commit.
PostgreSQL зберігає спостереження, assignment bindings, session generations
та поточні оцінки. Controller перераховує TTL щосекунди.

Перевірено на реальних процесах і PostgreSQL: unknown без доказів,
healthy після результатів двох Agent, відмова чужого CA, offline buffering,
restart, replay/deduplication, HTTP failure/recovery та Agent loss → unknown.
Окремий PostgreSQL-тест виконався з реальною БД й перевірив generation
fencing і конфлікт ID. Race-перевірки пакетів agent/secure/transport пройшли.

Повного protocol §9, enrollment/rotation/revocation, динамічного placement
та same-boot resume немає. Розподілений smoke — один хост і native-http;
реальний Blackbox перевірено окремим локальним smoke.
Докладно: `docs/architecture/distributed-slice.md`.

## Мережеві протоколи — реалізовано

Типізовані HTTP/TCP/DNS/ICMP checks, окремі decoder-и в registry,
валідація полів і типів config, Agent-side IR validation та capabilities.
Blackbox виконує TCP, DNS UDP/TCP з expectedRcodes/answer regex та ICMP.
IP allowlist застосовується до цілі або resolver; hostname targets не виконуються.
ICMP socket/permission failures не видаються за відмову ресурсу.

Перевірено real exporter: TCP success/refusal, DNS UDP/TCP, regex на
другому record, empty/NXDOMAIN та expected NXDOMAIN. ICMP success і
permission failure перевірено окремими ізольованими контейнерами.
Змішаний distributed smoke пройшов mTLS/PostgreSQL/replay/TTL.
Позитивний ICMP на хості недоступний через права; це не позначено успіхом.

## Ядро діагностики — реалізовано окремим компонентом

Gain/Cost selector, hypothesis filtering, actor eligibility/placement,
budget ledger, відсутність повторів Adaptive actions, Fixed DAG/CEL,
Static comparison mode, baseline equivalence matcher та пояснення рішень.
DiagnosticProfile parser використовує наявні kind/fields; підтримує
bounded subset; confirmation і bounded retry додано нижче; replicas>1 ще відхиляються.

Виконано синтетичний replay 15 runs у JSON/CSV з однаковими каталогами
та бюджетами. Це не мережевий експеримент. Контрприклад для early-stop
поза каталогом збережений і описаний у docs/architecture/diagnostic-core.md.
Перевірено формули, budgets/reuse, no-gain, technical failures,
baseline freshness/equivalence, placement, повторні Fixed steps і CEL.

Runtime Fixed/Adaptive підключено наступним етапом нижче. Публічного
запуску немає; bounded technical retry додано нижче; confirmation та натурні пілоти додані нижче.

## CEL Assessment — реалізовано в робочому ланцюгу

Inline assessment.rules з priority, bool type-check, контрольованим
контекстом checks/zone/target, cost/deadline та immutable compiled Assessor.
Правила входять до revision і виконуються Controller щосекунди.
Required failure не скасовується правилами; evaluation errors не
дозволяють healthy. Підтримується numeric integer metrics.httpStatus,
missing metrics залишаються відсутніми. Типізовані extraction/values додані нижче.

Unit-перевірки охоплюють priority, missing metrics, required overrides,
недопустимі поля й типи. `scripts/distributed_smoke.py --cel` відтворює
робочий mTLS/PostgreSQL ланцюг із resource down через CEL при probe success.
Докладно: docs/architecture/assessment-cel.md.

## Regex extraction — інтегровано

HTTP response.body → regex capture → string/integer/float/boolean → Observation
→ gRPC → spool/PostgreSQL → CEL values. Невідома capture не стає false/zero;
extraction error відділений від execution і probe outcome. Auto runner
обирає native-http; явний Blackbox з extraction відхиляється. Підтримано
bodyMatchesRegex для обох HTTP runner-ів як окрему перевірку probe.

Реальний distributed smoke `--extract` пройшов mTLS/PostgreSQL/replay,
HTTP 200 + ready=false → down, missing ready → unknown при повному
coverage, відновлення, Agent loss/TTL. Перевірено збереження int64 понад
2^53, float 1.0/1e100 і false через PostgreSQL jsonb та Protobuf. Native й
Blackbox smoke пройшли HTTP 200 з body mismatch → negative probe.
Докладно: docs/architecture/extraction.md. Raw body не зберігається.

## Автоматична діагностика Fixed/Adaptive — інтегровано

DiagnosticBinding → CEL trigger → Planner → durable reservation → mTLS task
→ Agent HTTP/Blackbox → spool → PostgreSQL → planner outcome/report.
Нормалізований DiagnosticProfile впливає на effective revision Monitor.
Heartbeat передає capacity та active gauge із session-local sequence.
Agent має незалежні regular/diagnostic lanes; місце результату в spool
резервується перед probe. Diagnostic observations не підміняють baseline.

Один запуск на епізод до healthy/full coverage — консервативний профіль.
Рестарт/зміна revision не відкривають новий бюджет. Crash активного запуску
дає inconclusive/controller_restart; reservations не повертаються. Результати
й останній звіт відновлюються з PostgreSQL; є single-Controller DB lock.

Live Adaptive перевірено з native HTTP, Fixed DAG — з реальними mTLS Agent.
Пройдено controller kill під час active task, recovery без повторного запуску,
coverage/TTL та PostgreSQL-тести ліміту й ізоляції diagnostic results.
Окремий live Adaptive smoke з реальним Blackbox Exporter також пройшов
виконання задач, crash/recovery, відновлення звіту й PostgreSQL-тести.
Докладно: docs/architecture/diagnostic-runtime.md.

## Перегляд результатів — приватний HTML-звіт

Controller за `-report-file` записує самодостатній read-only HTML; окрема
команда netmonorc-report формує такий файл із збережених JSON snapshots.
Видно зональні state/H/Q, причини/докази, режим, Agent, Gain/Score, outcomes
та reservations останньої діагностики. Попередня revision позначається;
поточний Assessment не підміняється старим діагнозом. Browser показує вік
оцінки й попереджає про застарілі дані. Файли приватні, додаткового HTTP API
немає. Це не React Web UI й не система керування конфігураціями.

## Static baseline — інтегровано в живий контур

Окремий comparison flag Controller запускає всі дії того самого каталогу
через той самий Runner та бюджет; нового DSL mode немає. Вибір входить у
revision. Live Blackbox smoke пройшов physical probes, diagnosis, budget,
restart і crash/recovery. Engine записує монотонно виміряний durationMs;
втрачене через crash вимірювання лишається відсутнім. Порівняння latency
на незалежному стенді й повторення для висновків дослідження ще не виконані.

## Compose-стенд — реалізовано

Окремі control/office/server bridges, один VIP ресурсу та різні маршрути
з двох Agent namespace. Контейнери Agent/Controller працюють без capabilities,
UID 10001; NET_ADMIN лишається в мережевих fixture. Образи та builder
зафіксовані digest. Приватні per-service config/key volumes заповнюються
через stdin; CA key не потрапляє в образи чи runtime Agent.

Реальний build і запуск пройшли з HTTP/TCP/UDP DNS та свіжим healthy з
обох Agent. Виправлено неявне використання HTTP proxy каналом gRPC і
ініціалізацію прав порожнього Docker data-volume. Команда up перевіряє
спостереження, а не лише стан процесу. Інструкція: docs/deployment/research-compose.md.

## Контрольовані натурні експерименти — пілот виконано

Runner перевіряє рівність компільованих каталогів/бюджетів, перемішує
порядок та експортує JSON/CSV з truth, Assessment, diagnosis, task records
і відомими completed probes. Primary: 15 запусків у двох логічних сегментах,
по одному повтору. Усі одиничні класи й зональна відмова підтверджені;
множинна HTTP+DNS дала два хибні класи у Fixed/Adaptive. Вони збережені.

State sampler очікує свіжі докази, не бажаний state. Попередній 15-run
route-withdrawal прогін з обмеженим sampler також збережено й окремо
позначено непридатним для оцінки state accuracy. Це не статистичний benchmark.
Agent loss на стенді дав office unknown/server healthy без diagnostic loop;
reconnect відновив healthy. Дані: docs/experiments/compose-pilot/README.md.

## Явне підтвердження — ядро й інтеграція реалізовані

Adaptive confirmation використовує всі actionRefs, count різних доказів,
Controller failure domains та causal minSpacing перед новим запуском.
Budget і TTL не обходяться; duplicated baseline ID не подвоюється.
Технічна невдача не повторюється як confirmation retry. Fixed із unknown
активованого кроку не видає completed. Нові default confirmed профілі
залишають original legacy конфігурації для відтворення попередніх даних.

Пройдено критичні Go/race перевірки faults, budget, domains, spacing/cancel,
expiry, duplicate baseline aliases та concurrent episode fencing. Погоджені
поля й консервативна семантика задокументовані у DSL/TЗ/DOCX та ADR 0001.
Реальний Adaptive smoke через mTLS/PostgreSQL/Blackbox підтвердив:
HTTP+DNS → inconclusive після двох primary і двох confirmation probes.
Повна серія із 15 запусків завершилася очікуваними state/diagnosis у всіх
семплах; HTTP+DNS → inconclusive в усіх режимах. Набір: docs/experiments/confirmation-pilot/README.md.
Це один повтор, не статистичний доказ універсальної точності.

## Час probe та доставлення задачі — розділено

20s orchestration allowance враховується всередині незмінного global
budget; check.timeout у driver не збільшується. Перед task поновлюється
30s lease, Agent перевіряє достатність safe time на повний probe/startup.
Live one-shot профіль явно обмежено 100ms..15s. Мінімум 6s більше не потрібний.

Окремий live DNS-drop пілот пройшов у трьох режимах: дві 10s перевірки DNS
завершились completed/failure, без execution_failed; отримано правильний
state/class. ADR 0002 і docs/experiments/dns-drop-pilot містять межі й дані.
Це не гарантія всіх мережевих таймаутів або підтримка OS suspend.

## Сповіщення — один AlertRule і HTTPS webhook інтегровані

Суворий decoder підмножини AlertRule/WebhookEndpoint, типізований CEL,
normal/pending/firing/resolved, for, окремий cooldown firing/resolved,
false→true rearm, reset pending при gap/revision/rule/restart.
PostgreSQL атомарно зберігає стан/перехід/outbox; attempt/lease й exponential
backoff із jitter переживають restart. Стабільний event_id, bounded attempts
та локальний TTL 24h; endpoint replacement скасовує старі недоставлені події.

HTTPS TLS verification, literal-IP allowlist, без redirect/proxy; bearer
лише з приватного файла. HTML і alert.json показують стан правила та enqueue.
Selector/zone scope/hostname webhook/public API не реалізовані. Документація:
docs/architecture/alert-delivery.md; рішення — ADR 0003.

Пройдено критичні FSM/TLS/negative config тести та race. Реальний Compose
прогін scripts/lab_notifications.py підтвердив 503→200 з тим самим UUID,
unknown при Agent loss без resolve, firing після Controller restart без
дубліката й доставку resolved після рестарту з durable queue. Обидві події
мали по дві фізичні HTTP спроби; receiver зберіг по одному logical event.
Повторюваний скрипт не експортує DSN/токени/ключі.

## Technical retry — реалізовано

Явне actions[].retry.maxAttempts (1…50, включно з першою), без неявних
повторів. Ланцюг тієї самої action × Agent активації повторюється лише після
execution_failed; negative probe/unknown не повторюються. Кожна спроба
витрачає незмінний global budget, має новий task ID і durable attempt_no.
Технічний результат не звужує гіпотези й не додає confirmation proof.
Fixed передає залежним крокам фінальний результат ланцюга.

Перевірено критичні budgets/deadline/cancel/no-repeat/domain тести й race.
Реальні Static, Fixed і Adaptive smoke через mTLS/Blackbox/PostgreSQL пройшли
startup failure → новий task з attemptNo=2 → completed/failure, правильний
клас, replay/TTL/generation fencing і crash без повернення reservations.
Деталі й межі: ADR 0004, docs/architecture/diagnostic-runtime.md.
Сирі дані всіх трьох режимів: docs/experiments/diagnostic-retry-pilot.

## Ще не реалізовано

1. Динамічне призначення, повний agent protocol і керування сертифікатами.
   Load/heartbeat і Compose мінімального профілю вже реалізовані.
2. Hostname targets і повна firewall-політика exporter.
3. CheckProfile/AssessmentPolicy refs і решта необхідних DSL
   можливостей; CEL assessment та CEL fixed conditions уже реалізовані.
4. Повторні сеанси зі спільним бюджетом, неперевірені
   unknown/multiple patterns, довгі tasks/suspend/backpressure. Явне
   confirmation, відома HTTP+DNS несправність та DNS silent timeout уже перевірені.
5. Повна інтерактивна вебпанель не входить до мінімуму. Перегляд результатів
   та стану алерту доступний через приватний HTML-звіт; простий webhook реалізовано.
6. Повторні серії експериментів і статистичний аналіз. Стенд із окремими
   сегментами, controlled faults та експорт уже реалізовані.

Цей стан не є відновленням попереднього коду. Погоджені confirmation поля
й уточнення внесено до DSL, ТЗ та DOCX із ADR 0001; HTTP API не змінено.
Непідтримувані можливості не позначаються завершеними.

## Експлуатаційне завершення: Linux lease і backup

Agent перевіряє process monotonic deadline та CLOCK_BOOTTIME перед новими
запусками й після startup exporter. Недоступний BOOTTIME закриває дозвіл;
restart завжди потребує нового assignment. Перевірено модель suspend clock
і реальний normal/reconnect/replay/TTL smoke. Фактичного suspend ОС не було.
Докладно: docs/architecture/suspend-aware-lease.md.

lab.py backup створює приватний pg_dump без перезапису старого файла.
Restore окремої тимчасової БД перевірено реально; робоча БД не змінювалась.
Порядок: docs/deployment/backup-recovery.md. Межі записки й приймання:
docs/planning/prototype-acceptance.md.

## Повторна натурна серія — завершена

30 запусків (2 повтори × 5 сценаріїв × 3 режими), без harness errors.
30 очікуваних states/results: 24 підтриманих класи, 6 multiple inconclusive.
Adaptive/Fixed по 4 probes, Static 6; перевага Adaptive над Fixed не доведена.
Raw evidence та автоматичний описовий звіт: docs/experiments/repeated-series.
CI додано й actions pinned; факт успішного remote workflow не перевірявся.

## Backpressure та складені несправності — перевірено

Реальна файлова spool на 1000 fixture records зупинила нові HTTP probes,
після ACK scheduler поновився; restart/drain зберегли всі 1000 pending
records. Race-перевірка пройшла. Це не 1000 фізично виконаних probes.
Опис: docs/architecture/backpressure.md.

Ще 9 live запусків HTTP+TCP/DNS+TCP/all-protocols у трьох режимах дали
очікуваний down та inconclusive без хибного визначеного класу. Дані:
docs/experiments/compound-series. Матеріал для остаточної звірки записки:
docs/thesis/implementation-alignment.md. Завантажений DOCX не змінено.

## Фактичний стан доставки у звіті — інтегровано

Controller показує scoped PostgreSQL snapshot pending/leased/delivered/
failed/cancelled і останню подію з attempts/HTTP status. Немає подій —
окреме повідомлення; disabled канал позначений. Enqueue/leased не означають
отримання HTTP-відповіді. Старі endpoint revisions і чужі rules не змішуються.
Додаткових HTTP routes/DSL fields немає, payload/URL/token не експонуються.

Реальні PostgreSQL isolation tests і live HTTPS retry/restart пройшли;
lab_notifications.py додатково дочекався delivered конкретного event ID
у HTML. Unit/race перевірили rendering/escaping та чужий Monitor.
Локальний offline report CLI без БД не показує цей додатковий блок.

## Демонстраційний пакет та офлайн delivery — реалізовано

Controller за -delivery-file записує приватний JSON snapshot доставки;
Compose й lab export включають його. netmonorc-report приймає -delivery
та відображає збережений стан без БД/HTTP. Чужий Monitor відхиляється.
lab.py package збирає лише дозволені snapshots, README та SHA256SUMS
у приватний tar.gz без перезапису попереднього файла. PKI/DSN/spool не входять.

Перевірено реальний package після HTTPS retry/restart, hashes, mode 0600,
offline rebuild та negative Monitor case; Go/vet і Python тести пройшли.
Документація: docs/deployment/demo-package.md; результат:
docs/experiments/demo-package. README актуалізовано: серії й webhook уже готові.
