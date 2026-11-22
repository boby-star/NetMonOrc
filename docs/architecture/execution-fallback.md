# ExecutionFallback: технічне резервування регулярних перевірок

Реалізує §7 `NetMonOrc_DSL_Schema_v1alpha1.md` у мінімальному live runtime.
Негативний результат цілі залишається доказом несправності цілі. Резервування
потрібне лише тоді, коли первинний виконавець технічно не виконав probe.

```yaml
fallback:
  enabled: true
  on: [agent_unreachable, runner_unavailable, local_execution_error, config_rejected]
  strategy: same-zone
  maxAttempts: 1
```

Блок належить `Monitor.spec.checks[]` або розгорнутому CheckProfile. `on`
обов'язковий і непорожній; дублікати та невідомі причини відхиляються. Default
strategy=same-zone, maxAttempts=1; допустимі 1..2 додаткові фізичні спроби.
`enabled: false` забороняє інші поля. Null, невідомі ключі та неправильні типи
не приймаються. Порядок причин канонізується; disabled/відсутній блок зберігає
старий Plan hash. Для live fallback timeout має бути <=15s через Agent lease.
Приклад повного [Monitor](../../examples/execution-fallback.yaml).

## Вибір виконавця та причина

Controller додає до effective revision повний довірений roster і відомі
failure domains. Це не metadata від Agent. Первинні джерела задаються звичайним
Monitor/check placement. Резервний Agent має бути окремим вільним джерелом,
яке ще не постачає цей check; capabilities, applied revision, heartbeat,
Component readiness, CIDR policy та спільне one-shot admission обов'язкові.
Вибір серед придатних кандидатів стабільний за Agent ID. DistinctBy=failureDomain
зберігає незалежність від решти запланованих replicas; невідомий домен не підходить.
Якщо check і так призначено всім Agents, резерву може не бути. Run лишається
pending до deadline, без вигаданого виконання й витрати фізичних спроб.
Прострочені roots переходять у durable expired обмеженими пакетами.

| Причина | Підстава |
|---|---|
| runner_unavailable | Свіжий первинний execution_failed/unknown: runner або exporter request unavailable |
| local_execution_error | Свіжий первинний execution_failed/unknown: локальна технічна помилка, некоректні метрики/читання результату |
| config_rejected | Agent відхилив саме видане assignment/revision; подія захищена поточною session generation |
| agent_unreachable | Немає свіжого applied heartbeat; startup grace 30s, останній результат старший interval+timeout+5s |

HTTP 503, DNS NXDOMAIN, TCP target timeout і невдалий body regex не запускають
fallback. Відмова target policy теж не є дозволом обійти policy іншим Agent.
Для config rejection/unreachable первинний фізичний результат може бути
невідомий: root містить причину й source, але порожній originObservationId.
Controller не створює за Agent синтетичний Observation. Успішне застосування
revision прибирає попередню відмову; вже підтверджене assignment не можна
відхилити заднім числом. Зміна session generation та rejection серіалізовані.

## Облік, виконання та покриття

Root містить Monitor/revision/check, первинний Agent/zone, причину, correlation
key, deadline та durable usedAttempts. За наявності первинного Observation його
ID є частиною кореляції. Для відсутнього виконання використовується часовий
інтервал check. Ліміт є на root, а не на весь інцидент: нова регулярна помилка
або наступний часовий інтервал може створити новий root.

PostgreSQL атомарно резервує task до відправлення, перевіряє budget, origin,
assignment/generation, deadline, точний нормалізований check та кандидата.
Повторний Agent у тому самому root заборонений; rejection/timeout не повертає
витрачений budget. Controller має два fallback workers і використовує спільний
з діагностикою one-shot slot Agent, залишаючи regular monitoring доступним.
Agent приймає лише точну одноразову форму свого check, довірені origin/zone та
дозволений attempt number. Receipt фіксується на диску до probe. Diagnostics
можуть бути вимкнені: fallback має власне явне повноваження в regular check.
Потрібна нова capability `fallback:v1` з обох боків; старий Agent не може
мовчки ігнорувати цей блок.

Кожен task має окремий Observation ID = task ID, fallbackTaskID та root link.
Physical Agent/zone не переписуються. Same-zone результат можна врахувати як
заміщення первинного джерела лише поки його первинний доказ не superseded
новішим regular execution. Для missing-origin перевіряється час останнього
regular result. Один physical Agent/check не може дати два незалежні голоси
для replicas. Звичайні freshness/skew/required/coverage guards зберігаються.
Different-zone результат доступний як інша перспектива, але не закриває
первинне покриття. Завершений негативний probe закриває root без наступного
fallback: це вже результат цілі.

При restart pending roots стають interrupted; їхні counters/tasks зберігаються
й не запускаються повторно. Пізній автентичний результат з дозволеним physical
часом може бути збережений. За часткової партиції старий Agent може ще виконувати
свій lease: гарантії exactly-once немає. Є bounded attempts та перевірювана
ідентичність кожного отриманого результату.

## Інтерфейси та підтверджені межі

- GET `/api/v1/fallback-runs?monitor=NAME&limit=50&cursor=ID` і `/ID`:
  read-only історія, scope resources:read, bounded query/time/concurrency.
- `netmonorc get fallbacks [ID]`: ті самі REST дані через чинний auth profile.
- React: історія, причина/стратегія/стан, лічильник, реальні виконавці та
  відкриття первинного/резервного Observation. Це не ручний запуск probe.
- Стартовий Monitor Controller компілюється через той самий resource registry,
  що й REST, включно з placement та посиланнями на доступні ресурси.

Натурно перевірені same-zone/different-zone, runner unavailable та відключення
основного Agent, HTTP 503 без fallback, справжні probes/БД/mTLS/HTTPS, відновлення
healthy і незмінність Monitor source version. Конфігураційні та локальні
технічні помилки мають реалізовані paths; окремих live fault trials усіх їх
варіантів немає. React пройшов production build, нових browser clicks не було.
[Докази](../experiments/execution-fallback-2026-11-20/README.md).

Допущені Controller same-zone fallback results тепер використовуються як
[початкові докази діагностики](fallback-baseline.md) зі збереженням фактичного
виконавця та run/task/origin provenance. Different-zone до baseline не входять.
Це не повна сертифікація старого ТЗ, HA/scale benchmark або доказ фізично
незалежних хостів.
