# YAML: детерміновані ревізії, керування перевірками та валідація

Підтримка на цій ітерації доповнює наявний DSL, не вводить нових назв полів.
Схема та повний продуктовий обсяг залишаються в NetMonOrc_DSL_Schema_v1alpha1.md.

## Приклад керування

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata: {name: controlled-http}
spec:
  enabled: true
  target: {type: endpoint, address: 192.0.2.10}
  checks:
    - id: application
      enabled: true
      type: http
      priority: 90
      jitter: 1s
      interval: 10s
      timeout: 2s
      config: {url: 'http://192.0.2.10/', expectedStatus: [200]}
    - id: optional-ping
      enabled: false
      type: icmp
      config: {}
  assessment:
    requiredZones: [office]
    checks: [{id: application, weight: 1, required: true}]
```

IP у прикладі документаційний. Для виконання потрібні власна дозволена ціль,
Controller roster та Agent з відповідними capabilities/CIDR policy.

- `Monitor.enabled: false`: нові regular probes не запускаються; Assessment
  стає unknown із причиною monitor_disabled, ресурс має status=disabled.
  Автоматичний і новий ручний запуск діагностики блокуються. Збережені
  Observation та звіти не видаляються. Повторне ввімкнення потребує нових
  придатних результатів чинної ревізії.
- `checks[].enabled: false`: перевірка проходить перевірку схеми/параметрів,
  але не входить до виконуваного плану або coverage. Явний assessment.checks
  має містити лише активні IDs; вимога disabled check є помилкою. IDs повинні
  бути унікальні навіть між увімкненими й вимкненими checks. У CheckProfile
  це правило застосовується після підстановки параметрів та об’єднання.
- `priority`: 1..100, default 50. Серед готових regular робіт Agent виконує
  вищий priority першим; далі старший due time, Monitor name та check ID.
  Це працює між Monitor, не змінює assessment weight і не перериває вже
  запущений probe. План залишається відсортованим за ID для канонічного hash.
- `jitter`: 0..interval/4, default 0s. Початковий запуск зміщується на [0,J],
  наступний планується від dispatch через interval плюс зміщення [0,J].
  Зміщення — SHA256 від Agent/revision/check/ordinal; однакові входи дають
  однакову послідовність. Лічильник належить scheduler process і починається
  заново після його перезапуску. Wall clock, фактичні мережеві затримки та
  черга залишаються реальними runtime входами, а не частиною обіцянки hash.

Задані явно стандартні enabled=true, priority=50, jitter=0s не додають нових
полів до історичного Plan JSON і не змінюють його ревізію. Нові non-default
параметри включені до hash та перевіряються Agent. Controller вимагає capability
`yaml:controls-v1`, якщо план її потребує; старий Agent не має мовчки ігнорувати
нові параметри. Priority/jitter не змінюють еквівалентність фізичного probe
для повторного використання регулярного baseline у діагностиці.

## Оцінювання та діагностика

Inline target приймає type/networkZone/criticality з валідацією. Стандартні
endpoint/unspecified/3 еквівалентні пропущеним значенням. Контекст використовується
CEL, але не створює інший placement і не розширює мережеві дозволи.

`assessment.weighted.enabled: false` дозволяє визначати оцінку через CEL
rules та defaultState. Доступні healthy/degraded/down/unknown; default unknown.
Fallback застосовується лише за придатних і достатніх доказів: required failure
має пріоритет, нестача доказів не стає healthy через default або нижче правило.
Ці ж поля підтримуються в повторно використовуваній AssessmentPolicy.

`adaptive.maxNoGainSteps`: 0..5. За default 0 зберігається зупинка при gain=0.
Явний додатний ліміт дозволяє не більше такої кількості додаткових активацій
із нульовою корисністю. Вони витрачають звичайний бюджет; не повторюють
неявно вже відвідані дії й не перетворюють either на доказ класу. Повторний
Run з тим самим episode ledger не відновлює використаний ліміт.

## Канонічні ревізії

DiagnosticProfileHash копіює та сортує семантично невпорядковані hypotheses,
actions, fixed steps, dependsOn, placement zones та confirmation actionRefs.
Зміна порядку цих колекцій не змінює effective revision і не мутує профіль.
Fixed placement agentRefs зберігають порядок: чинний planner використовує
його як перевагу виконавця. Зміни cost/expectations/budget/умов залишаються
змінами ревізії.

Після оновлення компілятора старий hash може канонізуватися один раз інакше.
LoadRuntime в одній транзакції оновлює похідні hashes профілів та залежних
Monitor, не переписуючи source manifest/generation/resourceVersion або
історичні сесії. Це також зберігає працездатність перевірки profile hash під
час нових ручних стартів. Перестановка source arrays може змінити source
resourceVersion/generation при PUT; гарантія тут стосується execution hash,
а не байтової тотожності початкового YAML.

## Один registry для offline та Controller validation

```sh
netmonorc validate -f bundle.yaml --slots branch=office,dc=server
cat bundle.yaml | netmonorc validate -f - --slots branch=office,dc=server
netmonorc validate --remote -f bundle.yaml --context-file /private/context.json
```

Локальна команда тепер читає ті самі multi-document YAML/JSON або ConfigBatch,
що й remote CLI, та викликає ResolveCatalog. Refs мають бути у переданому
комплекті; catalog Controller із мережі не завантажується. Порядок документів
не визначає результат; дублікати ресурсів/ключів, відсутні refs, cycles,
невалідні scalar types, regex, CEL та невідомі поля відхиляються.

Формат локального validate змінився: повертається validation summary, а не
виконуваний Monitor Plan. `mode=offline`, `planningRequired=true`; кожна
ревізія позначена validationRevision. Без --slots default requiredZones
перевіряються в явно позначеному abstractZoneContext=validation-only; такий
контекст не використовується для assignments. Для placement задається
статичний --slots, для WebhookEndpoint дозволи перевіряються через --allow-cidrs.
Remote validate/plan/apply використовують справжній persisted catalog і roster.

REST JSON також проходить структурну перевірку перед typed decode: duplicate
ключі на будь-якому рівні відхиляються до encoding/json overwrite. JSON
числа читаються без перетворення raw spec на float64. У compiler errors
зберігаються шлях вкладених полів/індексів і доступна line інформація.
Вхідні YAML/JSON — до 1 MiB; alias/depth/node budgets залишаються обмеженими.

## Фактичний залишок повної таблиці аудиту

Per-check placement/ref, Agent label selectors і distinctBy=failureDomain вже реалізовано:
[контракт і міграція](check-placement.md). Наведені зміни не означають реалізацію
всього старого ТЗ. [ExecutionFallback](../architecture/execution-fallback.md)
реалізовано з окремими корельованими physical attempts та збереженням
первинного доказу. Different-zone не відновлює первинне покриття.

[Паралельний Fixed DAG](../architecture/fixed-parallel.md) реалізовано зі
спільними квотами, stable wave journal, per-Agent admission та durable cap.

Component/external binding реалізовано: [контракт, підготовка модулів та
межі сумісності](components.md). Також реалізовано [proactive readiness
protocol](../architecture/component-readiness.md) з session/TTL/admission guards.

Live estimated-byte budget реалізовано з durable reservation та явною
[моделлю оцінки](diagnostic-byte-budget.md).

HTTP headers/connectIP/redirect та Agent-side DNS/IP policy реалізовано;
driver limits наведено в [контракті HTTP](http-policy.md).

Поля поза підтриманим контрактом й далі відхиляються там, де немає runtime семантики. Наявний
retry/domain confirmation/Blackbox runner не оголошується заміною цих функцій.
У registry тепер 11 kind; повну сумісність усіх полів DSL не заявлено.

## Стартовий каталог Controller

`-config` приймає [пакет пов’язаних YAML ресурсів](bootstrap-catalog.md).
Спільний із CLI decoder, атомарний seed графа, явний вибір основного Monitor
і збереження API-змін/tombstones при restart реалізовані без нових kind DSL.

[YAML-пакети React/REST](yaml-web.md) передають вихідний текст серверному decoder,
зберігають числові типи та використовують явні versions/requestId для apply.

[Одиночний редактор YAML/JSON](yaml-editor.md) отримує source та ETag одного
ресурсу разом; запис і перенесення в пакет зберігають точність чисел і версію
завантаженої чернетки.

[Monitor.spec.dependencies](monitor-dependencies.md) підтримує requires/soft
від Monitor до Target, канонізацію refs, спільний DAG і захист видалення.

[Статичні посилання CEL на checks](cel-check-references.md) перевіряються
після expansion до збереження desired; reusable AssessmentPolicy звіряє імена
при binding, а computed keys лишаються runtime CEL.

[Умови Fixed DAG](fixed-context.md) типізують task/monitor fields, перевіряють
статичні task refs за dependsOn та ізолюють predecessor context у runtime.

[DiagnosticBinding/Fixed/diagnostic placement](declarative-diagnostics.md):
типізований trigger, committed values/reasonCode та placementRef/selectors.
