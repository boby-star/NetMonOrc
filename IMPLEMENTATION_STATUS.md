# Фактичний стан мінімального прототипу

Це послідовність завершених етапів. Актуальні межі зведено наприкінці;
орієнтовна готовність мінімуму — [близько 80%](docs/planning/prototype-readiness.md).

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
bounded subset і явно відхиляє неготові confirmation/retry/replicas>1.

Виконано синтетичний replay 15 runs у JSON/CSV з однаковими каталогами
та бюджетами. Це не мережевий експеримент. Контрприклад для early-stop
поза каталогом збережений і описаний у docs/architecture/diagnostic-core.md.
Перевірено формули, budgets/reuse, no-gain, technical failures,
baseline freshness/equivalence, placement, повторні Fixed steps і CEL.

Runtime Fixed/Adaptive підключено наступним етапом нижче. Публічного
запуску, confirmation/retry та натурного порівняння трьох режимів немає.

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
Повна серія для трьох режимів і всіх п'яти сценаріїв ще перевіряється.

## Ще не реалізовано

1. Динамічне призначення, повний agent protocol і керування сертифікатами.
   Load/heartbeat і Compose мінімального профілю вже реалізовані.
2. Hostname targets і повна firewall-політика exporter.
3. CheckProfile/AssessmentPolicy refs і решта необхідних DSL
   можливостей; CEL assessment та CEL fixed conditions уже реалізовані.
4. Confirmation/retry, безпечна класифікація множинної несправності,
   мовчазні мережеві таймаути та повторні сеанси зі спільним бюджетом.
   Натурний пілот усіх трьох режимів уже є; помилки відкрито зафіксовані.
5. Простий webhook; повна інтерактивна вебпанель не входить до мінімуму.
   Перегляд результатів доступний через приватний HTML-звіт.
6. Повторні серії експериментів і статистичний аналіз. Стенд із окремими
   сегментами, controlled faults та експорт уже реалізовані.

Цей стан не є відновленням попереднього коду. Нормативні файли не змінені;
непідтримувані можливості не позначаються завершеними.
