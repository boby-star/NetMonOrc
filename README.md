# NetMonOrc

## Запуск із веб-панеллю

YAML: [короткі залежності Monitor](docs/deployment/monitor-dependencies.md),
[редактор одного ресурсу](docs/deployment/yaml-editor.md),
[пакети у React та REST](docs/deployment/yaml-web.md),
[стартовий каталог](docs/deployment/bootstrap-catalog.md),
[ExecutionFallback](docs/architecture/execution-fallback.md),
[Parallel Fixed DAG](docs/architecture/fixed-parallel.md),
[Component/external exporter](docs/deployment/components.md),
[HTTP policy](docs/deployment/http-policy.md),
[оціночний бюджет діагностики](docs/deployment/diagnostic-byte-budget.md),
[per-check placement](docs/deployment/check-placement.md),
[детермінованість і фактичний залишок](docs/deployment/yaml-controls.md).

[Покроковий quickstart](docs/deployment/quickstart.md): Docker build → prepare →
up --ui-port 8443 → admin з прихованим введенням пароля → https://localhost:8443.
Порт доступний лише на 127.0.0.1; дані зберігаються у named volumes.
Це робочий дослідницький профіль із [кількома Monitor](docs/deployment/multiple-monitors.md)
на одному Controller та спільних Agent. Повна відповідність
v1alpha1 ще не досягнута. Поточне доручення обмежене мінімальним дослідницьким
прототипом: [звірка записки й фактичний залишок](docs/planning/minimal-research-acceptance-2026-11-17.md),
[підтверджений запуск CLI/React та 15 fault trials](docs/experiments/acceptance-2026-11-17/README.md).

[Пояснення діагнозу](docs/deployment/diagnostic-explanation.md): журнал вибору
дій, ревізія, confirmation, JSON export та відкриття доказів із оцінок/алертів.

[Повний пакет діагностики](docs/deployment/diagnostic-export.md): CLI та React
збирають сесію, всі фізичні завдання й Observation для офлайн-аналізу.

[Відомі домени відмови](docs/deployment/failure-domains.md) задаються Controller
явно; невідомі домени не зараховуються як незалежні підтвердження.

## Поточний обсяг: REST API та React

REST/React інтегровані в дослідницький профіль; [розширений технічний план](docs/specs/thesis-scope-2026-11-13.md)
збережений як історичний документ і не розширює нинішній обсяг.
Реалізовано захищений Assessment REST, парольний вхід/вихід, durable
cookie-сесії, зміну власного пароля, історію перевірок і керування PAT у React.
CLI підтримує захищені login/whoami/local logout через особистий токен.
[Персональні токени та CLI](docs/deployment/personal-api-tokens.md).
Admin-панель підтримує [керування користувачами](docs/deployment/users.md)
із захистом останнього admin та конкурентних змін.
Додані [Monitor CRUD, редактор і реальне reconciliation](docs/deployment/monitor-resources.md):
збережена конфігурація доходить до агентів та визначає новий Assessment.
[Batch apply та operations](docs/deployment/batch-apply.md) підтримують атомарні
пакети, idempotency та підтвердження застосування агентами.
[Per-check placement](docs/deployment/check-placement.md): trusted Agent selectors,
failure domains, окремі джерела/coverage та історичний replay.
[YAML controls і валідація](docs/deployment/yaml-controls.md): enabled, priority/jitter,
канонічні діагностичні hashes та offline multi-document validate.
[Remote CLI](docs/deployment/remote-cli.md) має validate/plan/apply --wait
і читання Monitor/операцій через PAT.
[diagnostics.enabled](docs/deployment/diagnostic-toggle.md) перемикається
через apply зі збереженням регулярного моніторингу.
[Каталог DiagnosticProfile](docs/deployment/diagnostic-profiles.md) має CRUD,
React-редактор і mixed apply з підтвердженням нових залежних ревізій агентами.
[Граф залежностей](docs/deployment/dependency-graph.md): стрілки, вибір вузла,
пояснення relation/strength та сторінки каталогу в React.
[Dependency](docs/deployment/dependencies.md): перевірені посилання, requires DAG
та залежні ревізії без автоматичного перенесення відмов.
[ResourceGroup](docs/deployment/resource-groups.md): context/derived, membership,
weighted/coverage та окремий перегляд станів у React.
[Same-zone replicas](docs/deployment/replica-assessment.md): all/any/quorum,
окремі фізичні результати та coverage без вигаданих голосів.
[Динамічний PlacementPolicy](docs/deployment/dynamic-placement.md): leastLoaded/roundRobin,
persisted assignments/cursor, failover та pending без partial probes.
[Фіксований PlacementPolicy](docs/deployment/placement-policies.md): selection регулярних
агентів, pending без partial probes та окремий стан призначення у панелі.
[Спільні AssessmentPolicy](docs/deployment/assessment-policies.md): assessmentRef,
залежні ревізії, явні зональні SLA all-required/any та фактичний Target-контекст для CEL.
[CheckProfile і useProfiles](docs/deployment/check-profiles.md): типізовані шаблони
перевірок, безпечне розгортання й залежні ревізії Monitor.
[Спільні Target і targetRef](docs/deployment/targets.md): декларативні цілі,
залежні ревізії Monitor та підтвердження нової конфігурації агентами.
[Налаштування WebhookEndpoint](docs/deployment/webhook-resources.md) у REST/CLI/React:
збережена конфігурація, один активний канал і застосування з перевіркою credentials.
[Керування AlertRule](docs/deployment/alert-rules.md): REST/CLI/React, mixed apply
і виконання кількох збережених правил без перепідключення агентів.
[Деталі спостереження](docs/deployment/observation-detail.md) доступні за ID
через REST/CLI та з історії перевірок у React, включно зі старими ревізіями.
[Оглядова панель](docs/deployment/dashboard.md) показує свіжі стани ресурсів,
coverage, live Agent counts, алерти та останні діагностики з урахуванням scopes.
[Тест webhook](docs/deployment/webhook-test.md) проходить durable outbox
і має окрему Operation через API, React та CLI.
[Підтвердження алертів](docs/deployment/alert-acknowledgement.md) доступне
в API, React і CLI з захистом від застарілої версії та окремим receipt кожного циклу.
[Алерти й доставка](docs/deployment/alert-history.md) доступні для перегляду
через REST, React та CLI: докази, історія переходів і стан outbox.
[Ручна діагностика](docs/deployment/diagnostic-control.md) має start/cancel,
durable retry та best-effort зупинку фізичних задач без повернення бюджету.
[Історія діагностики](docs/deployment/diagnostic-history.md) показує сесії,
гіпотези, резерви бюджету й фізичні задачі через REST, React та CLI.
[Огляд агентів](docs/deployment/agent-snapshots.md) доступний у REST, React
та CLI get agents із поточних mTLS-сесій.
[Вхід і bootstrap адміністратора](docs/deployment/browser-sessions.md),
[історія та пагінація](docs/architecture/observation-history.md),
[перший етап REST](docs/architecture/rest-react-r1.md). Історичні відсотки
мінімального прототипу не характеризують готовність розширеного продукту.

## Мінімальний дослідницький профіль

Мінімальний дослідницький прототип методу комплексного моніторингу.
Робочий зріз — локальний ланцюг YAML → native HTTP / Blackbox → Observation → Assessment.
Повної відповідності попередньому v1alpha1 поки немає.

**Новий розподілений зріз:** окремі Controller/Agent, gRPC/mTLS,
lease, локальна черга та PostgreSQL. Запуск і межі реалізації:
[docs/architecture/distributed-slice.md](docs/architecture/distributed-slice.md).

## Compose-стенд із двома сегментами

[Інструкція запуску й контрольованих відмов](docs/deployment/research-compose.md).
Стенд не публікує порти й використовує приватні конфігураційні volumes.

## Запуск першого зрізу

Потрібні Go ≥1.25 і Python 3 для демонстраційного HTTP-сервера.

```bash
go mod download
go build -o .local/netmonorc ./cmd/netmonorc
python3 scripts/smoke.py
```

Демонстрація запускає справжній HTTP-сервер на `127.0.0.1:18080`,
виконує запити через зібраний CLI та перевіряє три результати:
усі успіхи → healthy; відсутня зона → unknown;
HTTP 503 у філії та HTTP 200 у серверній зоні → degraded.
Обидва джерела тут локальні: це **не** експеримент із незалежними мережевими зонами.

Команди окремо:

```bash
.local/netmonorc validate -config examples/http.yaml
.local/netmonorc probe -config examples/http.yaml -agent branch -zone office -allow-cidrs 127.0.0.1/32 > .local/office.ndjson
.local/netmonorc assess -config examples/http.yaml -slots branch=office,dc=server < .local/office.ndjson
```

Для `probe` потрібен доступний HTTP-сервіс із прикладу. Команда `assess`
читає NDJSON зі стандартного вводу; заплановані агенти задаються окремо,
тому втрата джерела не зменшує очікуване покриття.

## Blackbox Exporter

Підтримується підготовлений executable **Blackbox Exporter 0.27.0**.
Для HTTP check використовуйте `runner: blackbox` або `runner: auto`;
відсутній runner також обирає Blackbox відповідно до DSL.
CLI запускає один керований exporter на виклик `probe` із приватним
згенерованим конфігом і випадковим loopback-портом; при завершенні
процес і тимчасовий конфіг прибираються. Довільні аргументи, модулі
або executable paths у YAML не допускаються.

TCP, DNS (UDP/TCP) та ICMP також підтримуються через exporter:
[мережеві перевірки й запуск стенда](docs/architecture/network-probes.md).

```bash
python3 scripts/smoke.py --blackbox-binary /path/to/blackbox_exporter
.local/netmonorc probe -config .local/blackbox-smoke.yaml -agent branch -zone office -allow-cidrs 127.0.0.1/32 -blackbox-binary /path/to/blackbox_exporter
```

Для Cloud binary встановлено в
`/workspace/toolchains/blackbox_exporter-0.27.0.linux-amd64/blackbox_exporter`.
Офіційний Linux amd64 archive має SHA256
`507a77ff411822fd6b543e8b8d9e00e1cc49408df465588cd746753bf05046e2`.
Джерело: GitHub release `prometheus/blackbox_exporter`, tag `v0.27.0`.

HTTP runner-и приймають лише явні IP-literal URL з дозволених CIDR;
DNS-цілі, redirects, proxy та вимкнення перевірки TLS недоступні.
Це обмежена інтеграція з IP-literal targets, а не завершена мережева політика
для всіх протоколів. Startup exporter враховується в часі CLI-команди;
продуктивність майбутнього постійного Agent цим запуском не вимірюється.

## Межі поточної реалізації

- Один Monitor з inline target, HTTP/TCP/DNS/ICMP checks та inline weighted assessment.
- `native-http` та `blackbox`/`auto`, GET/HEAD, IP-literal URL, перевірка TLS,
  обмеження body/headers/timeout, явний CIDR allowlist, без redirects.
- Required checks, H/Q, TTL для кожної перевірки, maxSkew,
  поточна ревізія, newest observation, фіксований агент на зону,
  зональна агрегація all-required.
- Непідтримувані YAML-поля та можливості відхиляються.
- Це локальні CLI-операції, без нового альтернативного HTTP API.

Розподілений запуск підтримує фіксовані джерела, mTLS, послідовне регулярне
виконання й PostgreSQL. Наразі немає динамічного Placement/Scheduler,
повної інтерактивної вебпанелі. Один monitor-scope алерт та HTTPS webhook
із PostgreSQL outbox уже доступні: [доставка сповіщень](docs/architecture/alert-delivery.md). Для демонстрації доступний
[приватний HTML-звіт](docs/architecture/research-report.md). Fixed/Adaptive діагностика вже виконується через Agent:
[запуск і межі](docs/architecture/diagnostic-runtime.md).
Regex extraction з типізованими значеннями підтримується для native HTTP:
[семантика та приклад](docs/architecture/extraction.md).
Inline CEL assessment rules підтримуються:
[контекст, інваріанти та запуск](docs/architecture/assessment-cel.md).
CLI не автентифікує агентів: NDJSON придатний для контрольованого стенда,
а не для приймання недовірених мережевих результатів. Мережеві результати
приймає окремий gRPC Controller з перевіркою mTLS principal та assignment.

## Розробка

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
```

HTTP-тести й smoke потребують дозволу на локальні сокети.
У поточному Cloud компілятор встановлено в `/workspace/toolchains/go/bin`;
використовуйте цей PATH і writable кеш:

```bash
export PATH=/workspace/toolchains/go/bin:$PATH
export GOCACHE=/workspace/.cache/netmonorc-go
```

План і фактичний стан: [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).

Ядро Static/Fixed/Adaptive та синтетичний replay для дослідження:
[docs/architecture/diagnostic-core.md](docs/architecture/diagnostic-core.md).
Операційний запуск Fixed/Adaptive підтримує один консервативний сеанс на
інцидент, PostgreSQL budget reservations та відновлення після crash.
Static також доступний через прапорець дослідницького порівняння:
`-diagnostic-comparison-static`. Усі три режими виконують реальні probes,
є [пілот на двох сегментах із сирими даними й відомими помилками](docs/experiments/compose-pilot/README.md).
[Явне підтвердження](docs/experiments/confirmation-pilot/README.md) виправляє
відомий HTTP+DNS false class; [DNS-drop](docs/experiments/dns-drop-pilot/README.md)
перевіряє повний timeout. [Повторна серія 30 запусків](docs/experiments/repeated-series/README.md),
[9 складених сценаріїв](docs/experiments/compound-series/README.md) та
[HTTPS webhook](docs/architecture/alert-delivery.md) уже перевірені.

Орієнтовна готовність і залишок мінімуму: [критерії оцінки](docs/planning/prototype-readiness.md).

## Демонстраційний пакет

```bash
python3 scripts/lab.py package --output .local/netmonorc-demo.tar.gz
```

На запущеному стенді команда збирає приватні HTML/JSON snapshots та
SHA256SUMS без PKI, токенів або DSN. Файл не перезаписується. Це
демонстраційний пакет, не backup бази. [Перегляд і відтворення звіту](docs/deployment/demo-package.md).
