# Спільні AssessmentPolicy (R23, зональні SLA R25)

AssessmentPolicy підключено до наявного каталогу REST, CLI та React. Monitor
може використовувати assessmentRef замість inline assessment. Обидва одночасно
відхиляються. Відсутність обох використовує погоджені defaults і зони від
відомого Controller fixed roster. Без такого контексту requiredZones не
вигадуються: конфігурацію потрібно доповнити явно.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: AssessmentPolicy
metadata: {name: standard-web-assessment}
spec:
  requiredZones: [office, server]
  checks:
    - {id: http, weight: 1, required: true}
    - {id: tcp, weight: 1, required: true}
    - {id: dns, weight: 1, required: true}
  freshness: {maxAge: 20s, maxSkew: 15s}
  weighted: {enabled: true, healthyMin: 0.9, degradedMin: 0.5}
  defaultState: unknown
```

У Monitor замініть inline assessment на `assessmentRef: standard-web-assessment`.
Check IDs у політиці повинні відповідати фактичним checks після useProfiles
expansion, наприклад web__http. Якщо checks не задано, застосовуються рівні ваги
до всіх checks; required за замовчуванням false. Явний список повинен містити
кожний check рівно один раз; нульова сума ваг відхиляється.

## Семантика та поточні межі

Використовується той самий Assessment Engine: H/Q, required checks, TTL,
maxSkew та CEL. Підтверджена required-відмова дає down незалежно від H і
healthy CEL-правила. Брак доказів не стає healthy. Приклад із H=0.95 і
required failure перевірено unit test через реальний AssessmentPolicy ref.

Цей етап підключає reusable policy до вже підтриманого математичного профілю:
zoneAggregation=all-required або явний any, sameZoneAggregation=all,
weighted.enabled=true, defaultState=unknown. Replica voting any/quorum,
weighted=false та інші fallback states поки явно відхиляються.
Зональний any не є same-zone replica voting. Повні replicas/placement та
додаткові математичні режими залишаються окремою роботою. Формули не змінені.

Normalizer перевіряє закриту схему, defaults, weights, durations, правила й CEL
через чинний compiler. Внутрішні synthetic checks використовуються лише для
валідації самостійної політики: вони не потрапляють у runtime. При прив'язуванні
до Monitor повторно перевіряються справжні Check IDs, TTL/interval та fixed zones.
Відсутній maxAge обчислюється за фактичними checks, а не за validation example.

## Ревізії та операції

Monitor і AssessmentPolicy можна подати одним mixed apply незалежно від
порядку документів. Нормалізовані policy ref/hash входять до Monitoring IR
і checksum плану Agent. Policy-only update змінює effective revision усіх
залежних Monitor, залишаючи їхні source manifest/resourceVersion незмінними.
Plan показує implicit reconcile; завершення apply чекає завантаження нової
ревізії та heartbeat-backed ACK від потрібних Agent. Невибрані Monitor
очікують Controller selection і можуть залишати operation pending.

До появи придатних спостережень нової ревізії old assessment доступний лише
як історичний. Policy із Monitor-посиланнями не видаляється (409). Збережені
policy/refs завантажуються після restart і не перезаписуються bootstrap-файлом.
Startup Monitor у поточному prototype лишається одиночним inline bootstrap
з явними початковими зонами; reusable refs надходять через desired catalog.

```sh
netmonorc get --context-file /private/context.json AssessmentPolicy
netmonorc get --context-file /private/context.json AssessmentPolicy standard-web-assessment
netmonorc plan --context-file /private/context.json -f resources.yaml
netmonorc apply --context-file /private/context.json -f resources.yaml --wait
```

У React оберіть AssessmentPolicy в каталозі конфігурації. CRUD/history/status,
ETag/If-Match, scopes, cookie CSRF, audit й idempotency спільні з рештою ресурсів.
Status stored означає перевірений запис; activation підтверджує operation.

## Фактичний CEL-контекст Target

Для targetRef context містить дійсні type, networkZone, criticality і labels
Target. Вони включені у нормалізований IR. Зміна Target labels також змінює
ревізію залежного Monitor, якщо контекст впливає на оцінювання. Labels Monitor
не зливаються з Target labels, а target.networkZone не змінює zone.name джерела
перевірки. Criticality не стає вагою/severity автоматично: це лише факт для
явних CEL-правил. Inline address-only target зберігає наявний endpoint/
unspecified/criticality=3 context. IR потребує узгоджених Controller/Agent.

## Критичні перевірки

Pure tests: oneOf/ref/defaults, H=0.95 + required failure, missing evidence,
Agent VerifyPlan, policy hash і фактичний Target CEL context/label revisions.
PG/HTTPS: mixed apply, JSONB normalization, 428/409, implicit revisions,
незмінність source version та old healthy assessment fencing. Compose:
policy-only update дає healthy→degraded→healthy на двох Agent, переживає restart,
потім inline source відновлюється. Go/React build і go vet перевіряються;
DOM clicks цим сценарієм не автоматизовані.

## Явний SLA any та таблиця зон (R25)

`zoneAggregation: any` означає достатність здорової зони для доступності
сервісу. Це явний вибір адміністратора, а не majority чи заміна defaults.
Поле працює inline і через assessmentRef; CRUD, mixed apply, CLI та React
використовують той самий compiler. Наявні API routes/DTO не змінені.

| Зональні стани | all-required | any |
|---|---|---|
| healthy + healthy | healthy | healthy |
| healthy + down | degraded | healthy |
| healthy + unknown | unknown | healthy |
| down + unknown | degraded | degraded |
| down + down | down | down |
| degraded + unknown | degraded | degraded |
| unknown + unknown | unknown | unknown |

Це таблиця ТЗ §4.6. За будь-якого healthy у режимі any результат healthy;
без healthy наявність доведеного degraded/down дає degraded, якщо всі зони
не down. Зональні state/reason/evidence/H/Q і coverage не переписуються.
Відсутній Agent зменшує coverage навіть за global healthy. Негативна required
перевірка лишається локальним down; healthy CEL її не скасовує. У режимі
all-required down+unknown тепер коректно degraded: доведено локальну проблему,
але немає доказів глобальної відмови. Ця корекція не вводить нового DSL-поля.

Ваги, TTL/maxSkew, ревізії та minCoverage застосовуються в кожній зоні як
раніше. Any не може створити здорову зону з недостатніх локальних доказів.
Зональні alerts залишаються доступні, навіть коли global any SLA healthy;
автоматична діагностика керується явно заданим trigger, а не прихованою зміною
стану. У панелі оцінюйте загальний стан разом із coverage та списком зон.

Mode входить в IR/effective revision. Для all-required поле IR опускається,
щоб старі default revisions лишалися стабільні. Any вимагає capability
assessment:zone-any; неоновлені Agent відхиляються, а не ACK-ають незнайому
семантику. Controller та Agent оновлюються разом. Старі observations іншої
ревізії не застосовуються до нового SLA; policy-only update залишає Monitor
source resourceVersion незмінним та очікує належні Agent ACK.

Вузький сценарій `lab_sessions.py --zones-only` використовує новий приватний
owned stand і звичайний login. Він перевіряє actual office path failure,
порівняння режимів, відсутній Agent, REST coverage, restart і відновлення
обох required sources. Фактичний результат зберігається окремо після запуску;
DOM automation новим сценарієм не виконується.
