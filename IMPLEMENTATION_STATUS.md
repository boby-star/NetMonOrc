# Monitor.spec.dependencies: derived requires context

Додано короткий список Target refs за DSL §15.1: Monitor → Target,
requires/soft. Явний requires тієї самої пари має пріоритет без дублювання.
Спільний graph validator відхиляє dangling refs, неправильні типи/дублікати та
змішані cycles. Порядок списку не змінює revision, [] зберігає старий hash.
Delete Target захищений; похідні links не мають фіктивних ресурсів у БД.
Read-only compiled context доступний REST/CLI/React; OpenAPI містить новий
endpoint. [Контракт](docs/deployment/monitor-dependencies.md).

Фінальний browser trial підтвердив ACK Agent і healthy з реального probe,
граф із derived provenance, 409 на DELETE, atomic rejection змішаного cycle,
вилучення посилання/Target і повернення healthy. Dependency resources у БД=0;
Monitor source history=2. Production CLI прочитав порожній контекст після
вилучення. Go/vet і production build пройшли; screenshot переглянуто.
[Докази](docs/experiments/monitor-dependencies-2026-11-22/README.md).
Це закриває конкретну прогалину shorthand з попереднього аудиту, а не всі
залишки повного ТЗ чи весь OpenAPI contract.

# Одиночний YAML-редактор без втрати числових типів

GET/POST/PUT одного ресурсу повертають source, сформований сервером із тієї
самої View, що UID/version/ETag. React читає й записує текст YAML/JSON; Spec не
перетворюється на JS numbers. Prepare додає documents[] для перенесення до
ConfigBatch без округлення та зі збереженням завантаженої версії. CRUD YAML
обмежений одним manifest без batch conditions; URI/If-Match лишаються чинними.
[Контракт](docs/deployment/yaml-editor.md).

На справжньому браузері/HTTPS/mTLS/PostgreSQL підтверджено відкриття точних
1.0/9007199254740993, YAML validate/plan/save, незмінний effective hash,
копіювання YAML у пакет, 412 після сторонньої зміни та reload актуального source.
Go/vet і production build пройшли. [Докази](docs/experiments/yaml-editor-2026-11-22/README.md).
Це закриває зазначене нижче попереднє обмеження одиночного редактора.
Повну відповідність решті старого ТЗ не заявлено.

# YAML-пакети у React та REST

Пакетний редактор приймає multi-document YAML/ConfigBatch. Серверний prepare
розв’язує граф, заповнює лише відсутні versions/requestId і повертає текст
без створення ресурсів. Raw text проходить спільний decoder/compiler, без
браузерного перетворення Spec numbers. Canonical source зберігає 1.0 і великі
integer, повторний prepare — requestId/явні preconditions. YAML apply потребує
requestId; старий JSON ConfigBatch API сумісний. [Контракт](docs/deployment/yaml-web.md).

Натурний браузер підтвердив вхід, YAML validate/prepare/plan/apply, completed
Agent acknowledgements і healthy від фізичного probe. Повторний UI apply —
той самий operationId, рівно одна durable операція та source revision Monitor.
Перевірено exact integer у PostgreSQL, integer/float validation, duplicate key,
stale version 412, CSRF 403 та anonymous 401. Go/vet і production React build
пройшли; UI screenshot переглянуто. [Докази](docs/experiments/yaml-web-2026-11-20/README.md).

Редактор одного ресурсу й далі працює з JSON-поданням; точне перенесення
числових літералів у цій ітерації стосується пакетного редактора. Повну
відповідність решті старого ТЗ не заявлено.

# Атомарний стартовий YAML-каталог

Controller `-config` приймає 1..20 пов’язаних ресурсів у multi-document YAML
або ConfigBatch без apply preconditions. Спільний decoder CLI/Controller,
обмеження розміру/структури, явний `-monitor` при кількох Monitor, спільний
resolver посилань і одна транзакція seed/history/markers/revisions. Legacy
profile/alert/webhook files входять у ту саму транзакцію. Bundled webhook
використовує явні allowlist/secrets/CA, без потреби окремого endpoint file.
Збережені API-зміни мають пріоритет; tombstones не дозволяють resurrection.
[Контракт і приклад](docs/deployment/bootstrap-catalog.md).

Натурно підтверджено broken ref з 0 resources/history/markers; після виправлення
10 документів створено разом, два Monitor отримали healthy з реальних probes.
Shared Target змінено через REST, unused Target видалено; reverse-document
restart зберіг зміни, revisions та Monitor ETags, не додав source history.
Go/vet і production build пройшли; фінальний image повторно дав свіжі healthy
оцінки. Нових browser clicks не було: UI у цій ітерації не змінювався.
[Докази](docs/experiments/bootstrap-catalog-2026-11-20/README.md).

Це реалізація декларативного стартового каталогу в межах наявних kind DSL,
а не твердження про повну відповідність усьому старому ТЗ.

# ExecutionFallback та спільний compiler стартового Monitor

Реалізовано YAML fallback для регулярних checks: чотири технічні причини,
same-zone/different-zone, durable 1..2 додаткові спроби на кореляційний root,
точний check і trusted spare, receipt до probe, спільне one-shot admission.
Первинний доказ не переписується; unknown execution не отримує синтетичного
Observation. Same-zone може відновити джерело з чинним origin; different-zone
не закриває його coverage. Один Agent не створює два replica votes.
Read-only історія та proof links доступні через REST, CLI й React.
Controller bootstrap використовує resource registry, включно з placement/ref.
[Контракт та межі](docs/architecture/execution-fallback.md).

На двох ізольованих стендах підтверджено runner outage, відключення primary,
HTTP 503 без fallback, physical provenance, різне трактування зон, повернення
healthy та незмінність Monitor source version. Справжня PostgreSQL перевірила
ownership, no-refund, replay, config-rejection fencing і expiry. Go/race/vet
та production React build пройшли; фінальний image повторно запущено з обома
placement. Нових browser clicks не було.
[Докази](docs/experiments/execution-fallback-2026-11-20/README.md).

Цим закрито названий раніше блок ExecutionFallback; повна відповідність усім
полям старого ТЗ не заявлена. Live fault trials config_rejected та всіх
local_execution_error варіантів ще не проведені. Fallback results поки не
входять до автоматичного diagnostic baseline; вони мають окрему історію.
Наведені нижче розділи описують попередні ітерації.

# Parallel Fixed DAG

maxParallel>1 виконує незалежні DAG steps хвилями на різних Agents. Guards
бачать лише terminal snapshot попередньої хвилі; journal публікується стабільно
за Step ID, без залежності від фізичного порядку завершення. Count/bytes/retry
квоти спільні; shared admission зберігає regular slot, PostgreSQL атомарно
обмежує max_parallel. Новий stepId збережено у physical task/decision та
відображено REST/CLI/React. [Контракт](docs/architecture/fixed-parallel.md).

Натурно підтверджено два overlapping probes, join після обох, однаковий логічний
trace у двох запусках, count/byte exhaustion і скасування двох workers без child.
Справжня БД підтвердила atomic parallel reservation та незалежний byte cap.
Go/race/vet і production React build пройшли; browser clicks не було.
[Докази](docs/experiments/fixed-parallel-2026-11-20/README.md).

На момент цієї ітерації наступним був ExecutionFallback; його реалізацію
та фактичні межі наведено вище.

# Proactive Component readiness

Agent передає у heartbeat готовність exporter для точної Plan revision/ref/hash.
Controller відхиляє невірні/несвіжі звіти; reconnect не успадковує ready. Planner,
спільне admission та bound runner перевіряють свіжий статус перед diagnostic
reservation. Regular probes працюють і під час outage; native/local paths
збережено. REST/CLI agents та React показують Component state/time окремо від
Agent online і target health. [Контракт](docs/architecture/component-readiness.md).

Натурний mTLS/HTTPS/PostgreSQL сценарій підтвердив ready → unavailable → ready,
manual diagnostic під час outage із 0 додаткових checks і 0 physical tasks,
disabled/restart та відновлення healthy. Go/race/vet і production React build
пройшли; нових browser clicks не було.
[Докази](docs/experiments/component-readiness-2026-11-20/README.md).

На момент цієї ітерації залишався ExecutionFallback; реалізація описана вище. Parallel Fixed DAG
додано наступною ітерацією, описаною вище.
Готовність process endpoint не є підтвердженням завантаження всіх modules.

# Declarative Component / external Blackbox

Додано одинадцятий kind Component: closed YAML/JSON schema, canonical selector/
endpoint/defaults, trusted Agent selection, ambiguity rejection, effective Plan
binding hash, REST CRUD/validate/plan/apply/history, CLI get і React editor.
Production CLI component-config готує content-addressed modules із перевіреного
каталогу без мережі, install або exporter reload. Agent regular/diagnostic paths
використовують explicit binding без тихого повернення до локального binary.
[Контракт та межі сумісності](docs/deployment/components.md).

Натурний Controller–mTLS Agent–external exporter–PostgreSQL–HTTPS сценарій
підтвердив шість regular success, один реальний diagnostic task, outage unknown
без fallback, disabled unknown, restart зі сталою revision, відновлення healthy
і delete binding без зміни Monitor source version. Go/race/vet та production
React build пройшли; нових browser clicks не було.
[Докази](docs/experiments/components-2026-11-20/README.md).

Пізнішими ітераціями додано readiness та parallel DAG; ExecutionFallback описано вище. Unmatched Agents зберігають попередній локальний
prepared runner; це явна сумісність першого release, а не strict external-only
розгортання. Повної відповідності всіх полів DSL не заявлено.

# Live YAML estimated-byte budget

maxEstimatedBytes приймається live runtime. Coordinator відсікає дорогі
кандидати; PostgreSQL атомарно резервує estimate разом із фізичною спробою.
Retry/rejection не поповнюють бюджет. API/React і task export мають optional
estimatedBytes; старі run без ліміту не отримують вигаданих вимірів.
[Модель і межі](docs/deployment/diagnostic-byte-budget.md).

Натурний сценарій підтвердив нуль probes при недостатньому бюджеті,
рівно одну фізичну спробу при точному ліміті та збереження обліку після restart.
Реальна БД підтвердила atomic concurrent reservation/no-refund; Go/race/vet
і production React build пройшли. Browser clicks цього сценарію не було.
[Докази](docs/experiments/byte-budget-2026-11-19/README.md).

# HTTP YAML та перевірка фактичної адреси

Реалізовано connectIP зі збереженням Host/SNI/TLS verification, несекретні
headers, explicit redirects і per-socket CIDR policy. Auto статично вибирає
native для hostname/pin/redirect/extract; обмеження explicit Blackbox наведено
в [контракті](docs/deployment/http-policy.md). verifyTLS:false заборонено DSL.
Нові поля потребують capability Agent http:policy-v2.

Натурний Controller–Agent–PostgreSQL–HTTPS сценарій підтвердив healthy після
pinned redirect із правильними Host/header та unknown/target_policy_denied
для redirect на metadata IP. Початковий Monitor відновлено до healthy.
Go/race/vet, TLS socket tests і production build пройшли; browser clicks не було.
[Докази](docs/experiments/http-policy-2026-11-19/README.md).

# Per-check placement та trusted metadata

PlacementPolicy/inline placement підтримують matchLabels/matchExpressions і
failureDomain distinctness із Controller-owned inventory. Відомий домен не
виводиться із зони/ID. Check-level placement/ref працюють після expansion,
із власними persisted selection/cursor, scope, Agent dispatch та coverage.
Plan preview REST/React показує checkId. Historical allowed_checks у БД
захищають старий replay після нового runtime. Дані/volumes не видаляються.

Натурний сценарій підтвердив HTTP office / DNS+TCP server, healthy/complete,
restart без зміни revision, selector miss і distinct/shared/unknown domains.
Справжній CommitBatch відхилив чужий old-revision check і прийняв легітимний.
Go/race/vet, production React build пройшли; нових browser clicks не було.
[Контракт](docs/deployment/check-placement.md), [докази](docs/experiments/check-placement-2026-11-19/README.md).

# YAML controls, канонізація та єдина валідація

DiagnosticProfile hash більше не залежить від порядку actions/hypotheses/
steps/dependsOn; порядок fixed agentRefs збережено. Offline CLI validate
використовує registry, multi-document/ref resolution та stdin; видає validation
summary, не готовий виконуваний Plan. JSON duplicate keys блокуються до decode.
Додано inline target context, Monitor/check enabled, regular priority/jitter,
unweighted/defaultState з required/coverage guards і maxNoGainSteps=0..5.
Старі default Plan hashes збережено. Upgrade refresh не переписує source
version/generation/history. [Контракт і точний залишок](docs/deployment/yaml-controls.md).

Натурний mTLS/PG/HTTPS сценарій підтвердив незмінний hash після перестановки,
healthy/complete без disabled DNS, production CLI stdin validate, disabled
Monitor без probes і manual diagnostic, відновлення healthy. Окремий restart
підтвердив repair похідного hash без зміни джерела. Go/race/vet та production
React build пройшли; нових browser clicks не було. [Докази](docs/experiments/yaml-controls-2026-11-18/README.md).
Повну реалізацію решти полів старого ТЗ не оголошено.

# Початкові регулярні докази діагностики (§2.1–2.3)

Нові звіти зберігають baseline Observation з action/check відповідністю,
Agent/zone/domain, effective revision, physical time/TTL і результатом.
REST detail та React відкривають ці докази. CLI/React export включають їх
навіть для діагнозу без додаткових tasks; regular provenance перевіряється
окремо від diagnostic session/task. Legacy unknown не стає порожнім набором;
пакет має явний baselineEvidenceKnown. Бюджет/алгоритм оцінювання не змінено.

Натурний Controller–Agent–PostgreSQL–HTTPS сценарій підтвердив completed,
0 additional checks, 0 tasks, 1 regular proof та production CLI export
complete=true/baselineEvidenceKnown=true. Go/race/vet і production React
build пройшли; нових browser clicks не проводилось.
[Докази й межі](docs/experiments/baseline-evidence-2026-11-18/README.md).

# Часове вікно діагностичних доказів за §2.1–2.3

Runtime тепер застосовує Monitor TTL/maxSkew і до нових diagnostic results,
а не лише до початкових регулярних proofs. Transport передає фактичний
finishedAt durable Observation. Робочі hypotheses/support і confirmation
перевіряють latest-pair, TTL та coherent window, зберігаючи spent budget.
Нові звіти/API/React містять policy та physical timing; legacy час не вигадується.
Реальні probes дали completed при 20s/15s та inconclusive при 20s/5s і
10s/15s; у кожному сценарії spent checks=2, physical finishedAt збігся з
Observation. Go/race/vet та production React build пройшли; browser clicks
не проводились. [Натурні докази](docs/experiments/temporal-evidence-2026-11-18/README.md).
[Алгоритм і межі](docs/architecture/diagnostic-evidence-window.md).

# Controller-owned домени відмови за §2.1/§2.3 записки

Додано явний -failure-domains Agent=domain для trusted roster. Немає fallback
із zone/Agent ID: невідомі домени не підтверджують незалежність. Agent API/React
показує поточний домен; diagnostic decisions та confirmation зберігають домени
на момент висновку. Baseline до нового topology epoch не переінтерпретується.
Compose та distributed fixture задають свої логічні domain IDs явно.
Натурні HTTP-fault сценарії дали: distinct → completed/2 domains,
shared → inconclusive/1 domain, unknown → inconclusive/0 domains; по 4
reserved checks. Збережені domain snapshots збігаються між report та API.
Нові browser clicks не проводились, production React build пройшов.
[Натурні докази](docs/experiments/failure-domains-2026-11-18/README.md).
[Використання, сумісність і межі](docs/deployment/failure-domains.md).

# Резервування діагностичної місткості за §2.3 записки

Controller атомарно ділить один diagnostic slot Agent між Monitor до списання
бюджету. Очікування обмежене спільним deadline; після нього baseline expiry
перевіряється повторно. Cancellation не звільняє невизначений віддалений task
раніше result/deadline. Regular slot збережено. Agent завершує probe/exporter
до публікації результату, усунуто повторне скидання busy старим worker.

Реальний дво-Monitor/mTLS/PostgreSQL сценарій підтвердив два послідовні probes
на dc без busy-відмов, budget waiting 0/1, всього 2 використані спроби й
6 regular observations під час першого probe. Go/vet/race пройшли.
[Алгоритм і межі](docs/architecture/diagnostic-admission.md),
[натурні докази](docs/experiments/admission-2026-11-17/README.md).
Попередні записи про завершення описують стан на відповідній ітерації;
подальший аудит виявив цю прогалину координації, тепер виправлену.

# Програмний мінімум: фінальна інтеграція export та браузера підтверджена

Production CLI і React сформували повний пакет реальної діагностики:
4 tasks, 4 Observation, decision explanation/confirmation, complete=true.
Підтверджено також два Monitor, login/CRUD/session restore/logout; console
exceptions: 0. [Фінальний запуск](docs/experiments/diagnostic-export-2026-11-17/README.md).
[Стан завершення й явні межі](docs/development/readiness.md).
Залишок тексту магістерської записки не закривається програмним release.

# Повний експорт діагностичної сесії

Реалізовано `netmonorc diagnostics export` та React «Завантажити пакет доказів»:
усі сторінки фізичних tasks, точні Observation, session revision і decision
journal у schema=1 JSON. Збережено integer precision; 404 retention видимі
як missingObservationIds/complete=false; auth/transport помилки не приховуються.
Перевіряється session/task/Agent/Monitor provenance, зсув сесії й цикли paging.
CLI створює mode 0600 файл без перезапису, без часткового output при помилці.
Це послідовний evidence export, не атомарний database backup.

[Використання та межі](docs/deployment/diagnostic-export.md).

# Журнал діагностичних рішень та навігація доказами

Додано persisted explanation до деталей сесії REST/CLI/React: порядок
дій/Agent, attempts/phases, hypothesesBefore, gain/score, execution/outcome,
cost/duration, confirmation і blocked/skipped steps. Lists/Dashboard не
завантажують повний журнал; legacy без decisions позначається невідомим.
Сесія показує власну effective revision. React має JSON export та кероване
оновлення активної сесії без автоматичного повтору start/cancel.
Докази тепер відкриваються також із зонального Assessment та Alert.

Цільові Go, реальний PostgreSQL/HTTPS і production React build пройшли;
нового браузерного acceptance чи повторної fault-серії не проводилось.
[Контракт, використання та межі](docs/deployment/diagnostic-explanation.md).

# Приймання мінімального дослідницького профілю (17.11.2026)

Поточне доручення обмежує обсяг мінімальним дослідницьким прототипом із
Controller–Agent–Assessment–API–CLI–React. Історичний продуктовий залишок
нижче не є списком обов’язкових умов цього профілю.

Звірено нову редакцію записки із заповненим §2.3. Виправлено перебудову
hypotheses/support після expiry початкових доказів відповідно до алгоритму
2.2; нові докази можуть замінити старі без повернення витраченого бюджету.
У React фізичні diagnostic tasks відкривають збережений Observation із
Agent/zone/revision та session/task provenance через наявний API.

Remote підтвердив `4e60ed0`; локальний checkout без незакомічених змін
оновлено fast-forward з `c18bf87`. R33 enrollment-файлів у доступному дереві
та Git-історії не виявлено; їхній старий push/browser результат не підтверджено.
Preissued PKI залишається межею цього стенда.

Актуальні [вимоги й залишок](docs/planning/minimal-research-acceptance-2026-11-17.md)
та [свідчення запуску](docs/experiments/acceptance-2026-11-17/README.md).
Відсоток готовності магістерської роботи чи всього v1alpha1 не заявляється.
Нижче збережено історію попередніх ітерацій.

# Same-zone replicas та all/any/quorum (R32)

Controller підтримує кілька trusted Agent у зоні; regular placement дотримується
explicit distinctBy=zone і не створює partial наборів. Assessment бере latest
кожного Agent/check, planned-N voting, coherent time window та незалежний
required negative proof. Немає вигаданих votes із missing/execution failure або
replay. Q/H лишаються логічними; physical coverage та raw IDs збережені.

CEL працює з aggregate outcome/typed execution metadata. Values/metrics за
кількох same-zone replicas відхиляються checked-AST validation відповідно до
попереднього рішення користувача. Погоджені Q03 уточнення внесено в DSL §9.7
і ТЗ §4.5 (Markdown/DOCX); HTTP routes/DTO не змінено.

Критичні Go checks і vet пройшли. Реальний приватний Compose сценарій із двома
physical Agent однієї зони перевірив opposite outcomes, all/any/quorum tie,
missing vote, required failure без другого Agent та recovery. Browser test
і трьох-Agent live benchmark у цій ітерації не виконувались.
Evidence: docs/experiments/replica-assessment-r32/.
Межі: enrollment/cert lifecycle, trusted failureDomain/labels, per-check placement,
повний compiler-default, independent hot reload та load/storm acceptance відкриті.

# Динамічне regular placement (R31)

PlacementSpec/PlacementPolicy приймають leastLoaded та roundRobin; strategy
за замовчуванням у явному placement — leastLoaded. Trusted roster, online
heartbeat/capacity, capabilities і рекламований локальний CIDR allowlist є
hard filters. Повний feasible selection зберігається; втрата джерела запускає
новий відбір. Cursor на policy UID і selections на Monitor UID зберігаються
у PostgreSQL. Немає partial probes або нового права розширювати Agent policy.

Новий selection змінює effective revision, не source resourceVersion.
apply operations відстежують лише причинні placement transitions та чекають
ACK. Додано typed plan preview strategy; нових HTTP routes/DSL fields немає.
Critical Go, vet і реальний PostgreSQL пройшли; Compose integration
підтвердила roundRobin, restart/cursor persistence, failover, незмінну
ревізію сусіднього Monitor, pending без partial probes та default leastLoaded.
Evidence: docs/experiments/dynamic-placement-r31/.
Межі: fixed roster, один Agent на зону; legacy режим без placement/ref,
selectors/failureDomain/per-check overrides, same-zone voting, enrollment,
independent hot reload і storm/load acceptance ще відкриті.
Документація: docs/deployment/dynamic-placement.md; ADR 0012.

# Кілька Monitor у спільному runtime (R30)

Controller завантажує весь Monitor catalog, передає незалежні assignments через
одне mTLS-з'єднання Agent, оцінює кожний Monitor і має окремі diagnostic/alert
контексти. PostgreSQL зберігає ACK за revision/session generation; новий ACK
не скасовує інші. Operations перевіряють ACK усіх відповідних Monitor.
Agent розділяє checks із однаковим ID за revision та має один regular worker,
окремий diagnostic worker, bounded cache двох regular exporters і спільний spool.
REST routes збережені; React перемикає Monitor без нового входу, вибір вкладки
відновлюється після reload. Dashboard враховує всі актуальні assessments.

Критичні Go-перевірки, React/Docker build та ізольований реальний PostgreSQL
пройшли. Live browser integration пройшла: два Monitor із різними
станами, по два Agent, перемикання/reload React та видалення додаткового
Monitor без втрати здорового стану стартового. Evidence: docs/experiments/multiple-monitors-r30/. Зміни catalog усе ще викликають
спільний runtime reload/reconnect; незалежного hot reload немає. Dynamic
placement/replica voting, Agent lifecycle та повне приймання лишаються відкритими.
Документація: docs/deployment/multiple-monitors.md; ADR 0011.

# Локальний запуск UI та живий браузерний сценарій (R29)

Додано явний loopback-only HTTPS port для owned Compose stand і команду
початкового admin із прихованим вводом/захищеним stdin та idempotency.
Неінтерактивні Docker команди не успадковують stdin; SQL preflight більше
не споживає пароль. UI GET виконуються послідовно в межах вкладки та мають bounded retry
429/503/network у бюджеті 8s;
POST/PATCH/DELETE та auth errors не повторюються. Python checks, React build
і Chromium fixture retry cases пройшли. Live HTTPS browser acceptance пройшла:
login, cookie-session reload, Target/Dependency CRUD, graph, derived group
із реальних probes, reference guard, revision restore та logout. Перевірено
відсутність host publishing і UI network у приватних сервісів. Down/up
зберегли порт, admin і дані; після рестарту отримано healthy.
Evidence: docs/experiments/local-release-r29/.
Quickstart: docs/deployment/quickstart.md. Повний v1alpha1 не оголошується
завершеним: multi-Monitor, dynamic placement/replica voting, Agent lifecycle
залишаються великими відкритими блоками.

# Граф залежностей і стабільні компоненти панелі (R28)

React має read-only graph/table Dependency зі стрілками, вибором вузла/ребра,
клавіатурою, пошуком на поточній сторінці й server cursor pagination.
Схема не обчислює health/RCA. Виправлено duplicate sibling keys, які під час
clock updates дублювали секції та скидали вибір. Production build і критичні
Chromium DOM flows пройшли на fixture API; live Controller цим browser smoke
не перевірявся. Docker build пройшов; packaged assets побайтово відповідають
перевіреній браузером збірці. Evidence: docs/experiments/dependency-graph-r28/.
Документація: docs/deployment/dependency-graph.md.
Multi-Monitor, dynamic placement/replicas, Agent lifecycle та фінальна
acceptance/performance звірка залишаються відкритими.

# Dependency: перевірений граф і revisioned context (R27)

Окремий Dependency має CRUD/history/status, mixed apply, CLI та React editor.
Refs Target/Monitor, defaults, самопосилання та requires DAG перевіряються
перед commit. Зв'язки входять у релевантний Monitoring IR; зміна context
перебудовує effective revisions без зміни source Monitor version та чекає ACK.
Hard/soft не переносить health. Referenced delete захищено. Критичні Go/PG/HTTPS
перевірки, React build та вузький live workflow пройшли: cycle rejection,
Agent ACK/revisions, hard context без вигаданого failure, restart/delete.
Evidence: docs/experiments/dependencies-r27/. DOM clicks не тестувалися.
Документація: docs/deployment/dependencies.md. Shorthand Monitor.dependencies
не реалізовано через розбіжність DSL §4/§15.1; графічний DAG/RCA не додавався.
Великі відкриті блоки: multi-Monitor execution, dynamic placement/replica voting,
Agent lifecycle та фінальна acceptance/performance звірка.

# ResourceGroup: похідні стани й membership (R26)

ResourceGroup CRUD/history/status, members/labels selectors, context/derived,
all-required/any/weighted, mixed apply, implicit group revisions, REST assessment,
CLI та React підключено. Група не запускає probes; stale/old-revision/missing
MonitorAssessment стають unknown. Snapshot читається узгоджено в PostgreSQL.
Критичні Go/PG/HTTPS перевірки, React build та live group workflow пройшли:
missing member/coverage, selectors/implicit revisions, no extra probes, restart.
Evidence: docs/experiments/resource-groups-r26/. DOM clicks не виконувалися. Документація: docs/deployment/resource-groups.md.
Орієнтир залишку: 25–35% повного погодженого обсягу; метод оцінки та великі
відкриті блоки — docs/development/readiness.md. Multi-Monitor execution,
dynamic placement/replica voting, Dependency та Agent lifecycle не завершені.

# Зональний SLA any та коректне агрегування: R25

Підключено AssessmentPolicy.zoneAggregation=any через inline/ref, compiler,
IR, Agent validation/capability fencing та чинні REST/CLI/React інтерфейси.
Зональні докази/coverage зберігаються незалежно від global SLA. Виправлено
all-required down+unknown → degraded відповідно до ТЗ §4.6.
Критичні Go та PostgreSQL/HTTPS перевірки пройшли; вузький live сценарій
реального office path failure, Agent loss, REST coverage та restart пройшов.
Evidence: docs/experiments/zone-assessment-r25/. DOM automation не виконувалась.
Відкриті: dynamic placement, same-zone replica voting/value semantics,
multi-Monitor, groups/dependencies, enrollment/cert lifecycle та фінальна
acceptance/performance звірка. Повний v1alpha1 не завершений.

# Фіксований PlacementPolicy та regular assignments: R24

PlacementPolicy CRUD/history/status, placementRef/inline, mixed apply, CLI та
React підключені. Реалізовано strategy=fixed, deterministic selection із trusted
roster, zone filter, distinctBy=none/zone, replicas у різних зонах та pending
без partial probes. Hash/selection змінюють залежні effective revisions.
Необраний Agent ACK/heartbeat лишається online і може виконувати авторизовану
діагностику. Regular replay захищено historical assignment permissions.
Документація: [placement-policies.md](docs/deployment/placement-policies.md).
Критичні Go/PG/HTTPS перевірки та повний live Compose R24 пройшли:
selected/idle, diagnostic на idle, Agent loss→unknown, pending без partial,
policy-only revision та restart. React batch preview пройшов production build;
DOM clicks не автоматизовані. Evidence: docs/experiments/placement-r24/.
Відкриті: dynamic placement, same-zone replicas/full assessment modes,
multi-Monitor execution, groups/dependencies, enrollment/cert lifecycle,
UID lifecycle та фінальна acceptance/performance звірка.
Повний v1alpha1 не завершений.

# AssessmentPolicy refs та фактичний Target CEL context: R23

AssessmentPolicy CRUD/history/status, assessmentRef, mixed apply, CLI та React
підключені. Shared policy нормалізується й перевіряється чинним compiler;
зміна залежності оновлює Monitor effective revision без зміни manifest version.
Додано погоджені defaults для відсутнього assessment за відомого fixed roster.
CEL отримує фактичні Target descriptors/labels; їхні зміни revision-fenced.
Математичний профіль лишається all-required/all/weighted/unknown fallback;
непідтримані режими відхиляються явно.
Документація: [assessment-policies.md](docs/deployment/assessment-policies.md).
Відкриті: PlacementPolicy, multi-Monitor/replicas, повні assessment modes,
groups/dependencies, enrollment, UID lifecycle та фінальна звірка/експерименти.
Повний v1alpha1 не завершений.

# CheckProfile та типізоване розгортання перевірок: R22

CheckProfile CRUD/history/status, useProfiles, типізовані параметри/defaults,
as__id expansion, CLI та React підключені. Тільки whole-scalar config substitution;
зіткнення/наддовгі ID відхиляються. Hash профілю й bindings входять до IR;
profile-only changes оновлюють залежні Monitor без зміни manifest version.
Desired apply чекає залежні Agent ACK. YAML numbers не втрачають типи;
JSONB round trip не змінює declared-float hash.
Документація: [check-profiles.md](docs/deployment/check-profiles.md).
Відкриті: reusable policies, multi-Monitor placement/replicas, groups/dependencies,
enrollment, UID lifecycle та фінальна acceptance/performance звірка.
Повний v1alpha1 не завершений.

# Спільні цілі та наскрізні залежні ревізії: R21

Target CRUD/history/status, Monitor.targetRef, mixed apply, CLI і React
підключено. Нормалізований Target hash входить в IR/effective revision;
Target-only зміни перераховують залежні Monitor без зміни їх manifest version.
Apply очікує залежні Agent ACK; стара оцінка fenced. Passive logical Target
не створює probes і не оголошується healthy.
Документація: [targets.md](docs/deployment/targets.md).
Відкриті: CheckProfile/policies, multi-Monitor placement/replicas,
groups/dependencies, enrollment, UID lifecycle і фінальна звірка/експерименти.
Повний v1alpha1 не завершений.

# Збережена конфігурація webhook: R20

WebhookEndpoint CRUD/history/status, validate/plan/mixed apply, CLI та React
підключені. Один активний канал, inactive replacements, explicit CIDR/TLS policy
і локальний credential preflight; invalid batch не комітиться частково.
Controller завантажує desired із PostgreSQL та reconcile UID/hash; старі sender,
alert enqueue й webhook tests fenced. Readiness підтверджує runtime load.
Документація: [webhook-resources.md](docs/deployment/webhook-resources.md).
Відкриті: UID lifecycle, Target/CheckProfile/policies, multi-Monitor
placement/replicas, groups/dependencies, enrollment та фінальна звірка/експерименти.
Повний v1alpha1 не завершений.

# Керування правилами алертів і runtime integration: R19

AlertRule CRUD/history/status, validate/plan/mixed apply, CLI та React editor
підключені до PostgreSQL. Runtime виконує кілька правил вибраного Monitor;
CEL кешується за UID/hash. Старі rule/Monitor revisions fenced у транзакції
FSM/outbox. Apply готовий після оцінювання, не лише commit desired.
Документація: [alert-rules.md](docs/deployment/alert-rules.md).
Залишаються WebhookEndpoint CRUD, UID lifecycle, reusable Target/CheckProfile/
policies, multi-Monitor placement/replicas, groups/dependencies, enrollment
та фінальна acceptance/security/performance звірка. Повний v1alpha1 не завершений.

# Наскрізний перегляд доказів: R18

GET observations/{id}, CLI get observations ID та React «Відкрити доказ»
підключені до збережених результатів PostgreSQL. Історія та detail мають
спільну DTO проєкцію; збережені false/zero, стара ревізія та diagnostic
provenance не втрачаються. resources:read обов'язковий, raw body/config
не повертаються. Читання не впливає на live Assessment.
Документація: [observation-detail.md](docs/deployment/observation-detail.md).
Відкриті модулі повного v1alpha1 перераховані нижче; це не 100% реалізація.

# Огляд реальних джерел моніторингу: R17

GET dashboard/summary, React overview та CLI get dashboard підключено.
Monitor counts використовують спільні freshness/revision/coverage guards
детального Assessment. Невибрані/stale ресурси — unknown; Agent availability
окрема. Unauthorized fields не повертаються, recent diagnostics обмежені 5.
Документація: [dashboard.md](docs/deployment/dashboard.md).
Відкритий повний scope: rule/webhook CRUD, UID lifecycle, Target/CheckProfile/
policy refs, multi-Monitor placement/replicas, groups/dependencies, enrollment
і фінальна acceptance/security/performance звірка. Реалізація не є 100% v1alpha1.

# Durable webhook test та typed operations: R16

POST webhook/test підключено до API/React/CLI. Окрема подія без message
проходить чинні TLS/SSRF/retry/outbox, без fake Alert. Accepted означає commit
черги, completed — 2xx; статус переживає restart. Notification ledger мігрує
старі IDs/payload; operations читання фільтрує kind за відповідним scope.
Документація: [webhook-test.md](docs/deployment/webhook-test.md).
Відкриті: rule/webhook CRUD, UID lifecycle, multi-Monitor/placement,
Target/CheckProfile/policies, groups/dependencies та остаточна звірка вимог.

# Підтвердження поточного циклу алерта: R15

POST acknowledgement підключено до Controller/React/CLI; admin+alerts:manage,
CSRF і повторна перевірка actor у transaction. GET Alert має strong ETag,
If-Match обов’язковий для POST. Durable receipt/note окремі від FSM/outbox;
повтор не змінює перший час, новий цикл скидає поточне acknowledgedAt.
Документація: [alert-acknowledgement.md](docs/deployment/alert-acknowledgement.md).
Критичні PG/HTTPS та Compose сценарії й обмеження описані в доказах етапу.
Залишаються rule/webhook CRUD/test, UID-based registry lifecycle,
multi-Monitor, placement/replicas та решта DSL registry.
Нижче — історія попередніх етапів.

# Перегляд алертів, переходів і delivery summaries: R14

Додано GET alerts/list/get/history та webhook/deliveries, alerts:read,
bounded filter-bound cursors. Typed projection зберігається атомарно з
FSM/event/outbox; firing evidence не затирається unknown/stale даними,
legacy facts не вгадуються. React і CLI показують lifecycle та окрему
доставку без URL/payload/secrets. Alert FSM/retry формули не змінені.
Документація: [alert-history.md](docs/deployment/alert-history.md).
Залишаються acknowledgement, AlertRule/webhook management/test, UID-based
rule lifecycle, multi-Monitor, placement/replicas та решта DSL registry.
Нижче — історія попередніх етапів.

# Ручний запуск і скасування діагностики: R13

Підключені чинні POST start/cancel до Controller, React і CLI. Manual/auto
використовують спільні coordinator та durable incident/budget/cooldown.
Додано per-user 24h start idempotency; новий start за disabled/rollout заборонено.
Additive TaskCancel перериває поточну Agent probe best-effort. Cancelled state
і витрачений budget зберігаються за late receipt/result/planner completion.
Документація: [diagnostic-control.md](docs/deployment/diagnostic-control.md).
Критичний PG/HTTPS та Compose slow-probe сценарії описані в доказах етапу.
Межі, що лишаються: multi-Monitor/placement/replicas, решта reusable DSL kind,
ResourceGroup/Dependency, enrollment і повне керування alerts/webhook.
Нижче — історія попередніх етапів.

# Історія діагностичних сесій: R12

Підключені GET diagnostics/list/get/tasks, scope diagnostics:read, bounded
keyset pagination і monitor/state/time filters. DTO показують тільки
збережені дані: reservations окремо від Observation, hypotheses без звіту
не вгадуються, legacy profileRef не реконструюється. Додано React-розділ
та CLI get diagnostics/--tasks. Наявний research static mode відображено
в OpenAPI без зміни DSL або математичних алгоритмів.
Документація: [diagnostic-history.md](docs/deployment/diagnostic-history.md).
POST start/cancel та повне керування діагностикою залишаються відкритими;
так само multi-Monitor, placement та решта вказаного нижче обсягу.

# DiagnosticProfile registry та залежні ревізії: R11

Додано CRUD/status/revisions профілів через REST, React і CLI get;
PostgreSQL є джерелом актуальної конфігурації. Mixed apply перевіряє весь
об’єднаний граф у транзакції, незалежно від порядку документів. Зміна профілю
оновлює effective revision залежних Monitor без PUT їхніх маніфестів.
Операція включає implicit targets і чекає потрібних Agent ACK. Bootstrap
не перезаписує профілі й не відновлює видалені записи. Enabled references
захищають видалення; live runtime limits збережені.
Деталі й перевірки: [diagnostic-profiles.md](docs/deployment/diagnostic-profiles.md).
Залишаються multi-Monitor execution, решта DSL kind, динамічний placement,
enrollment, повні diagnostic/alert management API та решта розширеного обсягу.
Записи нижче описують попередні етапи, а не актуальний перелік пропусків.

# Read-only Agent snapshots: R10

GET agents/get-by-ID, React-таблиця та CLI get agents читають fixed roster
і актуальні mTLS-сесії. agents:read, online/offline/degraded, zone,
heartbeat/cert expiry/applied revision, nullable capacity/current probe gauge.
Read model не підміняє стан сервісу станом агента та не імітує enrollment.
Compose підтвердив scopes, API/CLI і справжній disconnect/reconnect.
Документація: [agent-snapshots.md](docs/deployment/agent-snapshots.md).

# Динамічне diagnostics.enabled: R9

Catalog profile availability відділено від дозволу запуску. PUT/apply активного
Monitor може вимкнути та повторно ввімкнути diagnostics, зберігаючи regular
checks та revision isolation. Static comparison guard збережений.
Окремий Compose/CLI сценарій підтвердив: fault при disabled не створює runs;
після re-enable fault запускає діагностику. Формули planner/budgets не змінені.
Деталі: [diagnostic-toggle.md](docs/deployment/diagnostic-toggle.md).

# CLI remote configuration workflow: R8

Додані validate --remote, plan, apply, apply --wait, get Monitor/operations.
YAML documents або JSON/YAML ConfigBatch, explicit versions/requestId,
private PAT context, shared bounded TLS client, status polling без POST retry.
CLI get зберігає JSON integer precision; HTTP error bodies не виводяться.
Compose перевірив YAML validation, plan, apply --wait до реальних ACK,
get completed після restart та зворотне застосування. Документація:
[remote-cli.md](docs/deployment/remote-cli.md). Загальні CLI kind/команди,
Cobra/named contexts та table/yaml output ще не завершені.

# Атомарний config/apply та durable operations: R7

Реалізовані POST config/apply, GET operations/list, React batch editor і
progress. Desired/ledger/Operation комітяться однією транзакцією; preconditions,
24h per-user idempotency, replay без повторного upsert, конфлікт reuse body.
Completed вимагає актуального stream ACK/heartbeat та generation fencing у БД.
Superseded targets завершують незакінчену операцію з failed; implicit prune немає.
Критичний PG/HTTPS та Compose apply→Agents→completed/restart/replay пройшли.
Документація: [batch-apply.md](docs/deployment/batch-apply.md).
Поточний scope — один active Monitor/fixed roster; інші kind і multi-Monitor
execution ще відкриті. Наступні записи — історія попередніх етапів.

# Monitor registry та реальне reconciliation: R6

Реалізовані Monitor CRUD, ETag/resourceVersion/generation, immutable revision
ledger, label filter, status, config validate/plan і React JSON-редактор.
PostgreSQL desired state переживає process restart; Controller bootstrap
не перезаписує збережений Monitor. Новий effective plan завантажується через
кероване перезавантаження runtime та reconnect/lease delivery агентам.
Live Assessment SQL gate виключає старий healthy під час переходу ревізій.
Критичний PostgreSQL/HTTPS сценарій та Compose REST → Agents → Assessment
пройшли: [докази](docs/experiments/monitor-resources-r6). Деталі: [monitor-resources.md](docs/deployment/monitor-resources.md).
Підтримано поточний один active Monitor і fixed roster. Інші DSL kind,
profile CRUD, multi-Monitor/replicas, atomic batch apply/operations та повний
CLI ще не завершені. Наступні записи — історія попередніх етапів.

# Керування користувачами: R5

Реалізовані Users GET/POST/GET-by-ID/PATCH та admin-розділ React.
Список без credentials, створення Argon2id, role/disabled, ETag/If-Match,
RBAC admin+system:admin, CSRF і повторна перевірка actor credential в транзакції.
Блокування/зміна ролі відкликає sessions/PAT; re-enable не відновлює їх.
DB transaction lock захищає останнього admin навіть за конкурентних mutations.
Невідомий createdAt legacy user не вигадується. Деталі:
[users.md](docs/deployment/users.md).
Критичні real PostgreSQL/HTTPS use cases та окремий production Compose
сценарій пройшли: [докази](docs/experiments/users-r5). React build і Go build/vet
проходять. Resource/config CRUD, загальні CLI commands, placement/replicas,
ResourceGroup та решта розширеного обсягу залишаються відкритими.

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
