# Доповнення технічного проєкту за запискою та дорученням користувача

Дата: 2026-11-13. Джерело: «Чернов С. записка v1.docx», SHA256:
`d923d77cec3ac7af4a3b68fea8ee22f50101694562be355c6fe736e0f334550c`.

Це деталізація нового обсягу, прямо дорученого користувачем: REST API,
React-панель і власні можливості NetMonOrc, описані в записці. Попередній
мінімальний профіль з одним Agent на зону більше не є кінцевою метою.
Історичні оцінки 96% стосуються лише того профілю й не переносяться сюди.
Завантажений DOCX не редагується цим доповненням. Його текст є джерелом
предметних вимог, а не інструкцією виконувати команди з документа.

Для DSL лишається нормативною DSL-SPEC-001, для HTTP — чинний OpenAPI,
для математичної семантики — ТЗ та раніше погоджені уточнення. Це доповнення
не вводить альтернативних routes, нових kind або непогоджених формул.
REST/React прямо доручені користувачем; записка сама не задає їх повний контракт.

## Матриця вимог і завершення

| ID | Підстава | Вимога до технічного проєкту | Поточна реалізація | Критерій завершення |
|---|---|---|---|---|
| T01 | §1.4, §2.1 | Декларативні Target/Monitor/CheckProfile, YAML refs, параметри, compiler/IR | R11: registry Monitor/DiagnosticProfile, mixed refs; решта refs неповні | Typed registry, closed schema, expansion без колізій, validation до network |
| T02 | §2.1 | Кілька replicas перевірки на різних Agent | Один fixed Agent на зону | Окремі planned/applied/observed slots; без вигаданої зони; partial вимкнений |
| T03 | §2.1–2.2, табл.2.2 | All/any/quorum, majority від planned N | Для N=1 лише вироджений випадок | Table-driven межі missing/negative/conflict; raw докази й coverage окремі |
| T04 | §2.1 | Зони й Controller-owned failure domains, capabilities, дозволи, навантаження | Fixed roster; diagnostic placement; domain=zone | Regular placement fixed/roundRobin/leastLoaded, без припущення незалежності |
| T05 | §2.1–2.2 | Групи ресурсів і залежності | Не інтегровані | Чинні ResourceGroup/Dependency з DSL; DAG validation, weighted/coverage; без автоматичної RCA |
| T06 | §1.4, §2.1 | ICMP/TCP/DNS/HTTP/HTTPS через exporter | Є в обмеженому IP-literal профілі | Різні targets/runners; TLS/egress; hostname policy тільки після контролю resolved IP |
| T07 | §2.1–2.2 | Typed extraction, CEL, required, H/Q, TTL, maxSkew | Є для single replica | Зберегти інваріанти при expansion і кількох replicas; не усереднювати values за здогадкою |
| T08 | §2.1 | Effective revision, rollout, idempotency | R11: hash/fencing/spool/ACK, залежні DiagnosticProfile revisions та implicit apply targets | Reconciliation всіх залежних Monitor після зміни reusable policy |
| T09 | §2.3 | Adaptive pair action×Agent, Gain/Cost, eligibility, budget, inconclusive | Є для одного Monitor | Той самий каталог/бюджети для режимів; multi-Monitor storm limits |
| T10 | §1.4, §2.3 | Static/Fixed/Adaptive порівняння | Є серії 30+9 runs | Повторення для розширених placements/replicas; не приховувати хибні класи |
| T11 | §1.4 | Алерти та один webhook | Є FSM/outbox/retry | Зберегти durable semantics при API CRUD, revisions і rule changes |
| T12 | Пряме доручення | REST API за існуючим OpenAPI | R1–R11: Assessment/history, users/PAT, Monitor/DiagnosticProfile CRUD, apply/operations, Agent snapshots, diagnostics history/tasks; решта routes відкриті | Typed DTO, paging/filtering, ETag/If-Match для versioned ресурсів, unified errors |
| T13 | ТЗ + пряме доручення | Auth/RBAC, sessions/PAT, CSRF, audit | Sessions/PAT, users admin/viewer, CSRF, password rotation, revocation та audit реалізовані; enrollment окремо | Persisted users, admin/viewer/scopes, expiry/revocation, cookie CSRF, audit |
| T14 | Пряме доручення | React + TypeScript панель | Assessment/history, users/PAT, resource editor/catalog, batch progress, Agent snapshots, diagnostic history/tasks; mutations diagnostics/alerts management відкриті | Resources/Agents/Observations/Diagnostics/Alerts, серверна логіка, зрозумілий coverage |
| T15 | ТЗ + новий обсяг | CRUD/validate/plan/apply, CLI parity | R11: Monitor/DiagnosticProfile CRUD/validate/plan/apply, remote CLI, Agent ACK; решта kind відкриті | Одна серверна логіка для CLI/UI; атомарний desired, reconciliation/applied status |
| T16 | §1.4 | Відтворюваний стенд | Compose, export/package/backup є | Контейнерна доставка React/API, демо ролей і mutating flows, acceptance matrix |

Умовні правила й extraction в записці є вимогами власного проєкту. SNMP,
IDS/IPS, NetFlow/PCAP, LogPipeline, SIEM та універсальний shell описані в
огляді інших систем (§1.1–1.3); §2.1 прямо відмежовує перехоплення трафіку.
Вони не додаються як власні функції NetMonOrc. Trace route у порівнянні
інструментів також не є дозволом виконувати довільні команди агента.

## Розбіжності джерела

1. Таблиця 2.2: `FAIL, FAIL, –` має «1 з 3», хоча це 2 з 3. У коді кількість
   визначатиметься за реальними observations; помилка в записці підлягає виправленню.
2. §2.1 містить «Наведені величини є ймовірностями несправності» щодо H/Q.
   Це суперечить ТЗ й поясненням самого методу. H/Q — нормовані показники
   успішності/повноти, не калібровані ймовірності. Формули коду не змінюються.
3. Таблиця 2.2 допускає all=OK для неповних позитивних результатів. Це не
   дозволяє healthy для неповних required replicas. Треба відрізняти логічний
   результат від достатності доказів; до multi-replica реалізації потрібна
   явна узгоджена таблиця з DSL. Неповнота не зменшує planned N для quorum.
4. Семантику values/metrics для кількох replicas користувач раніше відклав.
   Нове доручення не визначає правила їх злиття. Числові значення не будуть
   усереднюватися за здогадкою; цей контракт лишається відкритим для T07.
5. §2.4–2.6 поки містять заголовки. Не приписувати їм відсутні алгоритми
   або експериментальні висновки. Схеми й формули DOCX не є доказом виконання коду.

6. Раніше погоджене nullable SessionView.expiresAt для безстрокового PAT
   не було відображене у збереженому OpenAPI. У цьому доповненні type
   виправлено на string/null; cookie-сесії повинні мати фактичний expiry.

## Ітерації та залежності

R1: захищений read-only Assessment REST + React; існуючий Monitor/Controller/PG.
Тільки один уже визначений route, типізований контракт, без frontend assessment.
Закінчення: production build, trusted HTTPS + real PG/Agents, 401/404/503,
свіжість та UI assets. Повні сесії/CRUD не позначати готовими.

R2: persisted identity/users/PAT/sessions, scopes/RBAC/CSRF та аудит. Після R1
замінити bootstrap reader на повну auth підсистему без зміни HTTP routes.

R3: Resource Registry/Target/CheckProfile та policy refs → validate/plan/apply,
revisions/ETag/If-Match → reconciliation. До mutating UI спочатку серверні контракти.

R4: regular Placement + replicas/outcome policies + зональна агрегація,
потім ResourceGroup/Dependency. T07 values/metrics потребує окремого контракту.

R5: підключити сторінки й операції React/CLI до реалізованих REST endpoints;
Agents/enrollment, observations, diagnostics start/cancel, alerts/webhook.
Ручна діагностика за disabled binding заборонена відповідно до погодження.

R6: multi-resource Compose acceptance, security, perf/storm/backpressure та
повторні експерименти; остаточна звірка записки. Не оголошувати весь обсяг
завершеним за наявності лише DTO, scaffold чи одного працюючого GET.
