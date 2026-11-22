# Аудит декларативних сутностей, YAML та CEL

Перевірений код: `e2be554d68385c58cdc9bad11afc3902c6b24cfe`, гілка
`prototype/minimal-research`; локальний HEAD збігався з remote, дерево було чисте.
Джерела: `NetMonOrc_DSL_Schema_v1alpha1.md` (§1–2, 6, 8–13, 18, SC-18/19),
актуальна записка SHA256 `910615e8281e4978ffd1b1fe30cafb163b05b14057009e4cc7b41522bd0fd308`
(§2.1–2.4). Документ є джерелом вимог, а не інструкціями виконувати команди.

Висновок: усі 11 верхньорівневих kind присутні в закритому registry, але це
не повна реалізація всіх полів/контекстів DSL. Найбільші прогалини — спільний
CEL-контекст і DiagnosticAction placement. Парсинг YAML та regex extraction
працюють у явно обмеженому контракті. Нижче розділено дефект валідації,
відсутню функцію, погоджене обмеження і розбіжність специфікації з кодом.

## 1. Каталог kind

[Джерело переліку і compiler dispatch](../../internal/resources/catalog.go).
Для kind спільно існують ресурсний registry, versions/refs, REST/CLI та React
catalog/editing; наявність dispatch не є доказом реалізації кожного поля.

| kind | Фактично реалізовано | Межі/залишок |
|---|---|---|
| Target | Endpoint/logical target, адреса, labels/attributes, targetRef, контекст пасивного ресурсу | Attributes зберігаються, але target.attributes відсутній у CEL |
| Monitor | Inline/ref Target, checks/profiles, placement/assessment refs, diagnostics, dependencies shorthand, кілька Monitor, disabled | Не всі вкладені placement/CEL можливості однаково підтримані в діагностиці |
| CheckProfile | Типізовані parameters/defaults, scalar substitution, prefix expansion, перевірка конфліктів | Підстановки лише в дозволеному config; рекурсія/довільні шаблони заборонені за DSL |
| PlacementPolicy | Agent refs, trusted selectors, zones, replicas 1–10, distinctBy, fixed/leastLoaded/roundRobin | partial відхиляється; DiagnosticAction не використовує повний спільний PlacementSpec |
| AssessmentPolicy | Ваги/required, freshness/skew, H/Q, CEL, all/any/quorum, повторне використання | defaultState: healthy — розбіжність із DSL; values/metrics multi-replica відкладені |
| DiagnosticProfile | Fixed DAG, adaptive gain/cost, budgets, retry/confirmation, реальні tasks | Неповні CEL context та placement — див. нижче |
| AlertRule | Monitor ref, monitor-level when/resolveWhen, severity, for/cooldown, lifecycle/delivery | zone scope і monitor selector відсутні; вузький CEL context |
| ResourceGroup | Members/selectors, context/derived, all-required/any/weighted, coverage | Контекст/похідний стан; не окремий потік probes |
| Dependency | Typed Target/Monitor refs, relations/strength, requires DAG, read-only context | Не автоматична RCA; це відповідає контекстній ролі ресурсу |
| Component | Blackbox/external, trusted Agent selector, endpoint/moduleSet, binding/readiness | standard-network; managed/install/shell не входять у v1alpha1 |
| WebhookEndpoint | Централізований HTTPS endpoint, secretRef, timeout/retry, durable delivery | Один активний канал за контрактом; SMTP/довільні канали не є відсутніми kind |

Check, Extract, Fallback, AssessmentRule, DiagnosticAction, FixedStep, бюджети
і selectors — вкладені об'єкти. Agent, Enrollment, Observation, Assessment,
Session/Task, Alert/Delivery, User/Audit/Assignment — операційні об'єкти.
Їх відсутність у Supported(kind) правильна; додавати довільний Agent YAML
чи секрети до monitoring manifest не потрібно. Цей аудит не перевіряє повноту
операційного enrollment/PKI lifecycle.

## 2. YAML-парсер і вкладені об'єкти

[DecodeBatch](../../internal/resources/batch_decode.go),
[YAMLJSON/DecodeJSON](../../internal/resources/yaml_json.go),
[typed/closed validation](../../internal/config/probes.go),
[resource compilation](../../internal/resources/registry.go).

| Частина | Результат |
|---|---|
| YAML/JSON, multi-document, ConfigBatch | Реалізовані спільні decode/resolve; пакет 1–20 manifests, input ≤1 MiB |
| Unknown fields, duplicate/non-string keys, status/runtime metadata | Вхідні схеми закриті; негативні parser probes та наявні tests пройшли |
| Alias/nesting/expansion | YAMLJSON обмежує depth=64, budget=65536, цикли відхиляються; merge << заборонений |
| Числа/типи | Integer/float distinction, великі integers, finite numbers; порти/boolean не приймають неявні приведення. Це не твердження про довільну точність float: float64 лишається float64 |
| Params/refs/graph | Закриті typed parameters, whole-scalar substitution, expansion, dangling refs/cycles, version/requestId, atomic desired apply |
| Check/probe | HTTP(S), TCP, DNS, ICMP; runner/type/config validation, enabled/interval/timeout/jitter/priority, per-check placement |
| ExtractSpec | RE2 regex, named/numbered group, string/integer/float/boolean, missing/error, bounded body/values. Тільки native HTTP; invalid regex відхиляється до виконання |
| ExecutionFallback | Технічні причини, same/different-zone, bounded durable attempts, provenance; same-zone baseline повторно використовується |
| FixedStep/DAG | Цикли/dependencies, typed when, статичні task refs, predecessor isolation, bounded parallel execution |
| Діагностика помилок | Частина decoder errors узагальнюється до JSON-compatible YAML required; однаково повних field path/line/column на всіх входах немає |

Regex не є загальним script/parser engine: JSONPath/json-field, filesystem,
shell, Jinja чи env interpolation не реалізовані й не є вимогами базового
ExtractSpec. Наявний extractor виконує потрібний для записки regex parsing.

## 3. Фактична матриця CEL

| Місце | Реальний контекст | Чого бракує щодо §18 DSL |
|---|---|---|
| AssessmentRule.when | checks: outcome/executionStatus/reasonCode/values/metrics/ageSeconds; zone: name/coverage/labels; target: address/type/networkZone/criticality/labels | target.attributes; metrics реально заповнює httpStatus, не загальний набір latencyMs тощо; zone.labels зараз порожня map |
| DiagnosticBinding.when | monitor — map<string,string>: name/state/coverageState | Немає закритої перевірки назв полів; немає numeric weightedScore, labels, target |
| FixedStep.when | tasks[id]: outcome/executionStatus; monitor: state; прямі/транзитивні predecessor outcomes | tasks.values/reasonCode; checks, target, diagnostic counters; monitor.name/coverageState/labels/weightedScore |
| AlertRule.when/resolveWhen | Typed monitor: name/state/coverageState | zone/checks/target; monitor.labels/weightedScore; zone-scope rule загалом не підтриманий |

Це чотири різні місця CEL, хоча when/resolveWhen — два вирази AlertRule.
[Assessment](../../internal/monitor/rules.go),
[trigger](../../internal/diagnostics/trigger.go),
[Fixed](../../internal/config/diagnostics.go),
[Alert](../../internal/alerting/rule.go).
У §18 є і неузгодженість: загальна таблиця надає checks для Alert, окремі
рядки checks.* перелічують Assessment/Fixed. Точний Alert checks contract
потребує уточнення з явно обраною зоною; реалізації такого context зараз немає.

Є compile/type checks → bool, bounded cost=1024 та runtime cancellation;
Assessment/Trigger/Alert мають короткий evaluation timeout, Fixed використовує
контекст сесії. Assessment expressions ≤4096 bytes; Trigger/Fixed/Alert ≤2048,
хоча загальний DSL каже 4096. External I/O, shell і user-defined YAML functions
не зареєстровані. Відсутнє runtime значення не дорівнює false/success.

SC-18 виконано для статичних імен checks у AssessmentRule. Для Fixed статичні
імена tasks перевіряються за DAG; computed keys перевіряються runtime в
ізольованому predecessor context. Відсутність extracted value не є помилкою
самого YAML, а невідоме статичне поле typed object повинно нею бути.

### Підтверджені контрприклади

1. `CompileTrigger('monitor.sttae == "down"')` успішний, але виконання дає
   `no such key: sttae`. Це дефект ранньої валідації, а не відсутній Observation.
2. `monitor.weightedScore > 0.5` у trigger відхиляється через string/double:
   потрібний числовий context не реалізований.
3. `tasks["s01-dns"].values.depth`, `.reasonCode`, `checks["http"]` та
   `monitor.name` у Fixed відхиляються. Раннє відхилення коректне для наявної
   підмножини, але це не реалізація повного context DSL.
4. DiagnosticAction placementRef, selector, distinctBy відхиляються як unknown
   field; replicas=2 — explicit unsupported. У Monitor PlacementPolicy ці
   механізми (крім partial) реалізовані, тобто контракт PlacementSpec різний.
5. `defaultState: healthy` у Monitor приймається. DSL §9.1 це забороняє;
   код і існуючий test свідомо дозволяють healthy з runtime proof gates.
   Це розбіжність контракту, а не підтверджений bypass required/coverage.

## 4. Пріоритети залишку

1. **Виправлення валідації:** типізувати DiagnosticBinding monitor; невідомі
   поля відхиляти при validate. Уніфікувати оголошення й межі CEL context.
2. **Недостатні дані Fixed:** передавати values/reasonCode реальних завершених
   tasks із provenance та scalar types; додати задекларовані diagnostic
   counters і визначений monitor/target/checks context, не вигадуючи зону.
3. **DiagnosticAction placement:** placementRef і selectors/distinctBy зі
   спільним resolver; окремо визначити multi-replica execution/budget contract.
4. **Alerts:** monitor selector, zone scope та потрібний context. Погодити
   неоднозначність checks у загальній таблиці, не змішувати зони автоматично.
5. **Доповнення context:** target.attributes, monitor labels/score, реальні
   latency metrics та джерело zone labels; перевірити type/provenance/TTL.
6. **Узгодження:** defaultState: healthy, 2048/4096 limits, field-level schema
   матриця; повні field-path помилки. Partial placement і multi-replica values
   позначені як погоджені/відкладені обмеження, не приховане виконання.

## 5. Перевірки та практичні межі

24 executable probes: compiler, parser та bounded evaluation. Пройшли наявні
Go tests config/resources/monitor/diagnostics/alerting/probe/CLI. Нового live
Controller–Agent або браузерного запуску в цьому аудиті не було; попередні
приймання не перетворюються на нове підтвердження всіх полів.
[Код probes, JSON і лог](../experiments/yaml-cel-audit-2026-11-22/README.md).
Це звірка основних сутностей і ключових розбіжностей, не сертифікація кожного
поля кожного driver чи 100% відповідності всьому старому ТЗ. Runtime код у
межах цього аудиту не змінювався.
