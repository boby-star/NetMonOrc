# NetMonOrc — повна специфікація YAML DSL (`netmonorc.io/v1alpha1`)

**Документ:** `DSL-SPEC-001`  
**Статус:** затверджуваний архітектурний контракт / нормативний проєкт v1alpha1  
**Призначення:** єдине джерело вимог для реалізації parser → validator → compiler → scheduler → agent → assessment → diagnostics → alerting.  
**Мова:** українська. Імена полів, enum-значення, `kind` і назви типів у YAML наведено англійською та чутливі до регістру.

> **Статус реалізації.** Цей документ **проєктує** завершену схему DSL для першої дослідницької версії NetMonOrc; він не засвідчує, що відповідні конструкції вже реалізовані. «Повна» означає, що описано всі підтримувані в межах **цього контракту v1alpha1** типи ресурсів, їх поля, поведінку та межі. Нові типи у майбутньому вводяться шляхом розширення реєстрів і версіонування, а не мовчазної зміни семантики старих маніфестів.

## Навігація по документу

| Потрібно знайти | Див. розділ |
|---|---|
| Усі типи ресурсів і різницю між YAML та runtime | **§1–2** |
| Об'єкти мережі, Monitor, його вкладені checks | **§3–4** |
| Поля й типи ICMP/TCP/DNS/HTTP, runner | **§5** |
| Regex extraction, execution fallback, labels/placement | **§6–8** |
| Ваги результатів, coverage, правила станів | **§9** |
| Умовна та адаптивна діагностика, fixed DAG | **§10–11** |
| Повторно використовувані параметризовані профілі | **§12** |
| AlertRule, єдиний webhook | **§13–14** |
| Граф залежностей, пасивні й похідні групи, зовнішні компоненти | **§15–17** |
| CEL-контекст і runtime-об'єкти | **§18–19** |
| Міжресурсні перевірки, матрична модель | **§20–21** |
| Схема компіляції/застосування та приклади YAML | **§22–23** |
| Defaults, інсталяція, тести, модулі Go, джерела | **§24–29** |

---

## 0. Статус вимог і правила читання

| Позначення | Значення |
|---|---|
| **Обов.** | Поле обов'язкове, якщо не вказано альтернативу `oneOf` або умовну вимогу. |
| **Опц.** | Необов'язкове поле. |
| **Умов.** | Обов'язковість визначається типом ресурсу, драйвером, режимом або іншим полем. |
| **Default** | Значення, яке додає компілятор при відсутності поля; не дорівнює буквальному запису в YAML. |
| **Ref** | Символьне посилання на інший ресурс (`metadata.name`) відповідного `kind`; перевіряється семантичним валідатором. |
| **Runtime** | Дані, створені самим NetMonOrc. Користувач **не** задає їх у `spec` YAML. |
| **Поза v1alpha1** | Заплановане розширення, яке поточний компілятор має відхиляти, якщо воно з'являється в маніфесті. |

Синтаксис документа — **нормативний проєкт**, а не копія Kubernetes, Prometheus чи Sensu. NetMonOrc може використовувати їх як виконавців, але користувач пише **один власний DSL**, незалежний від синтаксису зовнішніх інструментів.

### 0.1. Межі версії

| Входить до v1alpha1 | Поза v1alpha1 |
|---|---|
| `Monitor`, `Target`, `CheckProfile`, `PlacementPolicy`, `AssessmentPolicy`, `DiagnosticProfile`, `AlertRule`, `ResourceGroup`, `Dependency`, `Component`, `WebhookEndpoint` | Централізований збір логів, `LogPipeline`, SIEM, NetFlow, універсальний SNMP-конфігуратор |
| ICMP, TCP, DNS, HTTP(S), `blackbox` та обмежений `native-http` | Довільні shell-команди, довільні мережеві драйвери та сторонні плагіни з YAML |
| Labels/selectors, ваги планування й оцінювання, зони, зональне покриття, CEL, regex extraction | Універсальна мова сценаріїв з циклами та виконанням довільного коду |
| Fixed diagnostic DAG, adaptive diagnostic candidate selection, обмеження бюджету | ML-прогнози, гарантоване визначення фізичної першопричини |
| Один вихідний webhook-канал для алертів | Декілька незалежних каналів і повноцінна система маршрутизації сповіщень |
| `Component` у режимі `external` (локальний exporter підготовлений Docker Compose) | Автоматичне встановлення довільних образів із маніфестів `Monitor` |
| YAML/CLI/API як одна ресурсна модель, read-only Web UI + базові операції адміністрування | Повноцінний GitOps-controller та візуальний low-code редактор всіх полів |

### 0.2. Ключові архітектурні інваріанти

1. `spec` описує **бажану конфігурацію моніторингу**, а не бажаний фізичний стан сервера. `status` визначає система.
2. Не змішуються **помилка виконання**, **негативний результат probe**, **відсутні дані** та **діагностичний висновок**.
3. Результати залишаються прив'язаними до агента, зони, часу, ревізії та конкретної перевірки; успіх у зоні `B` не стирає невдачу в `A`.
4. Зважені показники не замінюють обов'язкові checks і CEL-правила; `unknown` не перетворюється на нуль/успіх автоматично.
5. Звичайний `apply` **не встановлює нове ПЗ** та **не виконує shell-команди**. Установлення Master/Agent, початкового admin і enrollment — адміністративні операції за межами YAML DSL.
6. Регулярні checks локально виконуються агентами за призначеною ревізією; додаткові діагностичні завдання створює Controller.
7. Невідомі поля відхиляються; для нових можливостей використовуються зареєстровані типи/схеми та нові версії, а не довільні `map[string]any` в усіх місцях.

---

## 1. Каталог сутностей і відповідальностей

### 1.1. Декларативні ресурси (`kind`)

| `kind` | Призначення | Основні зв'язки | Активний / контекстний | Статус v1alpha1 |
|---|---|---|---|---|
| `Target` | Опис контрольованого або контекстного мережевого об'єкта | `Monitor.targetRef`, `Dependency` | Може існувати **без жодного Monitor** | Підтримується |
| `Monitor` | Композиція перевірок, цілі, призначення, оцінювання, діагностики | `Target`, `CheckProfile`, `PlacementPolicy`, `AssessmentPolicy`, `DiagnosticProfile`, `AlertRule` | Активний | Підтримується |
| `CheckProfile` | Повторно використовуваний набір параметризованих checks | `Monitor.useProfiles` | Конфігураційний | Підтримується |
| `PlacementPolicy` | Правила вибору агентів/зон і кількості replicas | `Monitor`, `Check`, `DiagnosticProfile` | Конфігураційний | Підтримується |
| `AssessmentPolicy` | Ваги результатів, обов'язковість checks, часові правила, зональна агрегація, CEL | `Monitor.assessmentRef` | Конфігураційний | Підтримується |
| `DiagnosticProfile` | Допустимі діагностичні дії, гіпотези, fixed DAG/adaptive selector, бюджети | `Monitor.diagnostics.profileRef` | Конфігураційний, запускається за умовою | Підтримується |
| `AlertRule` | Умови `pending/firing/resolved`, severity, затримка, політика доставки | `Monitor` за ref або labels, `WebhookEndpoint` | Конфігураційний | Підтримується |
| `ResourceGroup` | Групування Monitor за labels; опційно похідний стан групи | `Monitor.metadata.labels` | Контекстний або похідний | Підтримується |
| `Dependency` | Напрямлена семантична залежність двох об'єктів | `Target`/`Monitor` → `Target`/`Monitor` | Контекстний | Підтримується |
| `Component` | Прив'язка агента до вже доступного exporter/driver | `Agent` selectors, `Check.runner` | Конфігураційний, без інсталяції | Лише `external` |
| `WebhookEndpoint` | Єдиний централізований канал доставки подій алертів | `AlertRule.channelRef` | Конфігураційний | Один активний |

**Уточнення:** `Check`, `Placement`, `Extract`, `Fallback`, `AssessmentRule`, `DiagnosticAction` — **вкладені об'єкти**; вони не мають окремого `kind`. `Agent` не створюється довільним YAML: його ідентичність виникає лише після захищеної реєстрації.

### 1.2. Операційні об'єкти — **не** YAML-маніфести

| Об'єкт | Хто створює | Поля/сенс | Чому не задається YAML |
|---|---|---|---|
| `User`, `UserSession` | Auth/Bootstrap | username, passwordHash, role, session expiry | Створення першого admin — інсталяція; секрети не зберігаються в конфігах моніторингу |
| `AgentEnrollment` | Master UI/CLI | очікуваний IP (опційно), випадковий токен, TTL `20m`, consumed/revoked | Одноразова security-операція, а не desired monitoring state |
| `Agent` | Enrollment + Registry | agentID, сертифікат, capabilities, zone/failureDomain, health, labels | Ідентичність прив'язана до mTLS, не до YAML чи IP |
| `ConfigRevision` / `Assignment` | Resource Controller/Placement | desired/assigned/applied revision, agentID, checkID | Вираховується з декларацій, має історію застосування |
| `Observation` | Agent + Ingestion | результати probe, час, зона, ревізія, provenance | Фактичні дані виконання |
| `ZoneAssessment` / `MonitorAssessment` | Assessment Engine | зональний стан, підсумковий стан, coverage, evidence | Похідні обчислювані результати |
| `DiagnosticSession` / `DiagnosticTask` | Diagnostic Planner | епізод, виконані дії, обмеження, підсумок | Динамічний процес після trigger |
| `Alert` / `NotificationDelivery` | Alert Engine / Webhook Sender | firing/resolved, dedup, delivery attempts | Фактичні стани та доставка |
| `AuditEvent` | Система | автор, дія, час, ревізія | Незмінювана історія адміністративних дій |

---

## 2. Загальний формат кожного YAML-маніфесту

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata:
  name: corporate-portal
  labels:
    environment: production
    site: eu-west
  annotations:
    description: Main business application
spec: {}
```

| Шлях | Тип | Обов. | Default | Призначення, обмеження |
|---|---|---|---|---|
| `apiVersion` | string | Так | — | Точне значення `netmonorc.io/v1alpha1`. Незнайома версія — помилка, без тихої міграції. |
| `kind` | enum | Так | — | Одне зі значень каталогу §1.1; чутливе до регістру. |
| `metadata` | object | Так | — | Незмінний ідентифікатор за `kind + name` і метадані. |
| `metadata.name` | string | Так | — | `[a-z][a-z0-9-]{0,62}`; має закінчуватися буквено-цифровим символом; 1–63 символи. Унікальний у межах `kind` на інсталяції. |
| `metadata.labels` | map<string,string> | Ні | `{}` | Короткі `key:value` для селекторів/груп; кожен ключ і значення ≤63 символів. Не довіряти самоопису агентів для безпеки. |
| `metadata.annotations` | map<string,string> | Ні | `{}` | Описова інформація, не впливає на планувальник, якщо явно не визначено іншого правила. |
| `spec` | object | Так | — | Поля залежать від `kind`; **закрита** схема. |
| `status` | object | **Ні, заборонено** | — | Read-only у відповіді API/CLI, не у вхідному маніфесті. |
| `metadata.uid`, `metadata.generation`, `metadata.resourceVersion` | string/integer | **Заборонено** | Runtime | Генерує сервер для API та optimistic concurrency. |

**Номенклатура:** для назв ресурсів — kebab-case; для назв полів — camelCase; для `kind` — PascalCase; `labels` — `lowercase`, дефіс/крапка дозволені в ключах за окремим валідатором. Ідентифікатори вкладених checks/actions: `[a-z][a-z0-9_-]{0,63}`; префікс профілю формує ідентифікатори через `__`.

**YAML як формат:** YAML 1.2-подібні scalar values, багатодокументні файли через `---`, тривалість як рядок (`30s`, `5m`, `200ms`), дата/час RFC 3339 UTC для runtime. Дублікат ключа, неоднозначний тип, заборонений `NaN/Inf`, циклічний YAML anchor та невідоме поле — помилка. Не виконувати env substitution, shell substitution або Jinja-шаблони. Непередбачене приведення рядків `yes/no/on/off` у bool — заборонено; bool записується `true/false`.

### 2.1. Єдині типи й діапазони

| Тип | Формат | Валідація |
|---|---|---|
| `Duration` | рядок із `ms`, `s`, `m`, `h` (`30s`, `1m`, `500ms`) | Додатний, якщо інше не дозволено; без дробових значень у v1alpha1; максимум визначено кожним полем. |
| `Name` | рядок | За правилом `metadata.name`. |
| `CheckID` | рядок | `[a-z][a-z0-9_-]{0,63}`; унікальний після розгортання профілів. |
| `IPAddress` | IPv4/IPv6 literal | Без масок, без URL; нормалізація. |
| `CIDR` | IP-префікс | Валідна нотація IPv4/IPv6. |
| `URL` | `http://` або `https://` | Без userinfo, фрагментів і секретів у рядку; перевіряти DNS/redirect/IP за agent policy. |
| `Port` | integer | 1…65535. |
| `Weight` | number | 0…1; не `NaN/Inf`; нормування за політикою. |
| `Priority` | integer | 1…100; більше значення — вищий пріоритет. |
| `CELExpression` | string | Компіляція/типізація під час `validate`, лише дозволені змінні, функції й ліміти обчислення. |
| `Regex` | string | Go `regexp` / RE2-сумісний синтаксис, без backreferences/lookbehind; обмеження розміру шаблону й вхідного body. |
| `Ref` | string | `metadata.name` ресурсу потрібного `kind`; перевірка існування/циклів. |

### 2.2. Правила посилань, пріоритетів і merge

| Ситуація | Однозначна семантика |
|---|---|
| `...Ref` і однойменний inline-блок одночасно | **Помилка** (`oneOf`). |
| `Monitor.spec.targetRef` і `target` | Рівно один із них. |
| `Monitor.spec.assessmentRef` і `assessment` | Рівно один із них. |
| `Monitor.spec.placementRef` і `placement` | Не одночасно. Відсутність обох → стандартна `PlacementPolicy` компілятора. |
| `Check.placementRef` / `Check.placement` | Перевизначає placement у `Monitor` для цього check; `...Ref` та inline не разом. |
| `CheckProfile` + inline checks | Об'єднання **за ID лише після** підстановки параметрів; колізія ID → помилка, автоматичного overwrite немає. |
| `apply` того самого нормалізованого ресурсу | Ідемпотентно: нова ревізія не створюється без реальної зміни семантичного `spec`. |
| Відсутній Ref | Відхилити до виконання. |
| Невідомі `kind`, enum або поле | Відхилити з точним YAML path та причиною. |
| `delete` ресурсу з посиланнями | За замовчуванням відхилити, доки залежні ресурси не видалено/оновлено. |

---
## 3. `Target` — мережевий ресурс, включно з пасивним

**Сенс:** описує сам об'єкт інфраструктури, який може існувати незалежно від активного моніторингу. Створення `Target` **не** створює регулярні checks. Для об'єкта без адресу або Monitor можливий лише контекстний стан `unknown` або похідні висновки інших об'єктів; він не стає `healthy` автоматично.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Target
metadata:
  name: eu-gateway
  labels: {site: eu-west, role: gateway, environment: production}
spec:
  type: network-device
  address: 10.20.0.1
  networkZone: eu-west
  criticality: 4
  description: Основний шлюз сегмента EU
  attributes:
    provider: example-isp
    deviceRole: edge-router
```

| Шлях у `spec` | Тип | Обов. | Default | Мета / правила |
|---|---|---|---|---|
| `type` | enum `host`, `network-device`, `service`, `endpoint`, `logical` | Так | — | Класифікація ресурсу, не еквівалент OSI-рівню. |
| `address` | IP або FQDN | Умов. | — | Адреса для probes; для `logical` необов'язкова, для решти обов'язкова. Не дозволяється URL зі схемою — URL міститься у конфігурації `http`. |
| `networkZone` | string | Ні | `unspecified` | Де розміщена **ціль**; не тотожне зоні, звідки її перевіряють. |
| `criticality` | integer 1…5 | Ні | `3` | Відносна важливість ресурсу для планування / таблиці пріоритетів. Не означає severity автоматично. |
| `description` | string ≤1024 | Ні | `""` | Людинозрозумілий опис. |
| `attributes` | map<string, scalar> | Ні | `{}` | Обмежені описові атрибути (`string`, `integer`, `number`, `boolean`), max 32 ключі, не містять секретів і не запускають логіку. |

**Зв'язки:** `Monitor.targetRef`, `Dependency.from/to`. Labels у `Target` описують ціль; labels у `Monitor` описують сам монітор. Вони **не** зливаються мовчки: при потребі UI та CEL показують їх окремо.

---

## 4. `Monitor` — центральний декларативний ресурс

**Сенс:** визначає, **що** спостерігати, **які** перевірки потрібні, **звідки** їх виконувати, **як** оцінювати результат і **коли** призначати діагностику. Один Monitor може містити L3/L4/L7 checks і використовувати контекстні ресурси.

### 4.1. Верхній рівень `Monitor.spec`

| Поле | Тип | Обов. | Default | Призначення / правила |
|---|---|---|---|---|
| `targetRef` | Ref → `Target` | Умов. | — | Посилання на існуючий `Target`; **oneOf** із `target`. |
| `target` | `InlineTarget` | Умов. | — | Адреса цілі прямо у Monitor; **oneOf** із `targetRef`. |
| `enabled` | boolean | Ні | `true` | Якщо `false`, нові регулярні запуски не призначаються, поточна історія зберігається. |
| `useProfiles` | array<`ProfileUsage`> | Ні | `[]` | Підключення параметризованих `CheckProfile`. |
| `checks` | array<`CheckSpec`> | Ні | `[]` | Inline-перевірки. У сумі з профілями потрібен хоча б 1 check для активного Monitor. |
| `placementRef` | Ref → `PlacementPolicy` | Ні | — | Загальна політика розміщення; oneOf із `placement`. |
| `placement` | `PlacementSpec` | Ні | — | Inline-політика. За відсутності — один допустимий найменш завантажений агент. |
| `assessmentRef` | Ref → `AssessmentPolicy` | Умов. | — | Посилання на політику оцінювання; oneOf із `assessment`. |
| `assessment` | `AssessmentSpec` | Умов. | — | Inline-політика оцінювання; якщо немає обох, вбудований **задокументований** default, §9. |
| `diagnostics` | `DiagnosticBinding` | Ні | `enabled: false` | Умовний запуск поглибленої діагностики; не визначає глобальний стан самостійно. |
| `dependencies` | array<Ref → `Target`> | Ні | `[]` | Зручний короткий запис відношення `requires` від inline/linked target; компілюється в граф залежностей; без автоматичного успадкування стану. Для інших типів відношень — окремий `Dependency`. |
| `description` | string ≤1024 | Ні | `""` | Коментар про монітор, без виконуваної семантики. |

### 4.2. `InlineTarget`

| Поле | Тип | Обов. | Default | Значення |
|---|---|---|---|---|
| `type` | enum типів `Target` | Ні | `endpoint` | Тип цілі. |
| `address` | IP/FQDN | Так | — | Адреса базового ресурсу. |
| `networkZone` | string | Ні | `unspecified` | Розташування цілі. |
| `criticality` | integer 1…5 | Ні | `3` | Важливість для планування. |
| `attributes` | map<string,scalar> | Ні | `{}` | Допоміжний контекст. |

Inline Target є частиною Monitor і **не створює** окремий ресурс `Target`. Якщо потрібні спільні залежності між багатьма Monitor — краще оголосити самостійний `Target` та використати `targetRef`.

### 4.3. `ProfileUsage` — використання профілю

| Поле | Тип | Обов. | Default | Значення |
|---|---|---|---|---|
| `ref` | Ref → `CheckProfile` | Так | — | Який reusable набір перевірок підключити. |
| `as` | CheckID-подібний префікс | Так | — | Простір ідентифікаторів екземпляра. ID `https` з профілю стане `web__https`. |
| `parameters` | map<string, typed scalar> | Умов. | `{}` | Значення формальних параметрів профілю; обов'язкові параметри мають бути передані, решта отримують default. |

**Правило шаблонів:** всередині `CheckProfile.spec.checks[].config` дозволена лише **цілісна scalar-підстановка** `${params.<name>}`. Не дозволяється інтерполяція шматка URL, CEL-виразу, regex чи довільного YAML; підставлене значення зберігає оголошений тип. Синтаксис `${params.url}` за межами CheckProfile відхиляється. Результат підстановки перевіряється звичайним типізованим валідатором check.

### 4.4. `CheckSpec` — вкладений опис регулярної перевірки

| Поле | Тип | Обов. | Default | Призначення / обмеження |
|---|---|---|---|---|
| `id` | CheckID | Так | — | Унікальний у скомпільованому Monitor. |
| `type` | enum `icmp`,`tcp`,`dns`,`http` | Так | — | Тип виконання. HTTPS задається `type: http` із URL `https://...`. |
| `enabled` | boolean | Ні | `true` | Вимкнений check зберігається в конфігурації, не виконується та не зараховується до запланованого coverage, якщо оцінювання явно не вимагає його (у такому разі конфігурація невалідна). |
| `runner` | enum `auto`,`blackbox`,`native-http` | Ні | `auto` | `auto` обирає `blackbox` для звичайних probes; `native-http` — якщо потрібен `extract` або `connectIP`. `native-http` дозволений лише для `http`. |
| `interval` | Duration | Ні | `30s` | Періодичність, від `10s` до `24h`. |
| `timeout` | Duration | Ні | `5s` | Час виконання від `100ms` до `60s`, строго менше `interval`. |
| `jitter` | Duration | Ні | `0s` | Випадкове зміщення часу запуску, `0 ≤ jitter ≤ interval/4`. |
| `priority` | integer 1…100 | Ні | `50` | **Вага/пріоритет планування**, не оцінки здоров'я. Вищий пріоритет — раніше за дефіциту слотів. |
| `config` | типізований `ProbeConfig` | Так | — | Поля визначає `type` і можливості `runner` (див. §5). |
| `placementRef` | Ref → `PlacementPolicy` | Ні | — | Перевизначення загального розміщення; oneOf із `placement`. |
| `placement` | `PlacementSpec` | Ні | — | Inline-політика лише для цього check. |
| `extract` | array<`ExtractSpec`> | Ні | `[]` | Тільки для `type: http`, `runner: native-http` (або `auto`, що обере native); див. §6. |
| `fallback` | `ExecutionFallback` | Ні | `enabled: false` | Лише технічний fallback, не приховування негативного результату цільового probe; §7. |

**Перевірка типів:** жоден невідомий `type` або не зареєстрований `config`-параметр не дозволяється. Всі runner-specific параметри перевіряються **до** збереження desired revision. Зміна `type` в існуючому check змінює його конфігураційну семантику та створює нову ревізію.

---

## 5. Каталог `ProbeConfig`: усі параметри підтримуваних checks

### 5.1. ICMP (`type: icmp`)

```yaml
- id: reachability
  type: icmp
  runner: blackbox
  interval: 30s
  config:
    preferredIPProtocol: ip4
    ipProtocolFallback: false
    payloadSize: 32
```

| `config.*` | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `preferredIPProtocol` | `ip4` / `ip6` | Ні | `ip4` | Яку сім'ю адрес перевіряти спочатку. |
| `ipProtocolFallback` | bool | Ні | `false` | Чи дозволяти перехід до альтернативної сім'ї. Для відтворюваних вимірювань за замовчуванням вимкнено. |
| `payloadSize` | integer | Ні | `32` | Корисне навантаження ICMP, 0…1400 байтів. |
| `dontFragment` | bool | Ні | `false` | Лише для IPv4 за наявності підтримки runner і необхідних мережевих прав. |

`Target.address` має бути IP/FQDN. ICMP failure може означати фільтрацію ICMP і не є безумовним доказом недоступності прикладного сервісу. Blackbox Exporter на VPS може вимагати `CAP_NET_RAW` лише для відповідного контейнера.

### 5.2. TCP (`type: tcp`)

```yaml
- id: tcp443
  type: tcp
  config:
    port: 443
    preferredIPProtocol: ip4
    ipProtocolFallback: false
```

| `config.*` | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `port` | integer 1…65535 | Так | — | Порт цільового TCP endpoint. |
| `preferredIPProtocol` | `ip4` / `ip6` | Ні | `ip4` | Пріоритет адресної сім'ї. |
| `ipProtocolFallback` | bool | Ні | `false` | Чи можливий перехід до другої сім'ї. |

`TCP success` підтверджує встановлення з'єднання, але не працездатність HTTP/application. Для TLS- та application-level перевірки використовувати `http` із `https://`.

### 5.3. DNS (`type: dns`)

```yaml
- id: public-dns
  type: dns
  config:
    resolver: 192.0.2.53:53
    queryName: api.example.com
    recordType: A
    transport: udp
    expectedRcodes: [NOERROR]
    expectedRecordRegex: '^api\\.example\\.com\\..*'
```

| `config.*` | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `resolver` | DNS resolver endpoint `IP:port` / `[IPv6]:port` | Так | — | **Куди** спрямувати DNS-запит; не слід плутати з ресурсом, ім'я якого перевіряють. Лише IP literal + порт. |
| `queryName` | FQDN | Так | — | Яке DNS-ім'я запитати. |
| `recordType` | `A`,`AAAA`,`CNAME`,`MX`,`TXT`,`NS` | Ні | `A` | Тип запиту. |
| `transport` | `udp`,`tcp` | Ні | `udp` | Транспорт DNS. |
| `expectedRcodes` | array<string> | Ні | `[NOERROR]` | Допустимі DNS response codes, наприклад `NOERROR`, `NXDOMAIN`. |
| `expectedRecordRegex` | regex | Ні | — | Додаткова валідація answer section; не повертає capture groups як окремі `extract`-значення. |

Якщо потрібно обійти несправний resolver або визначити локальність DNS-помилки, створюють окрему перевірку з **іншим** `resolver` або діагностичну дію. Проблема DNS відповіді не доводить фізичну недоступність хоста.

### 5.4. HTTP(S) (`type: http`)

```yaml
- id: https
  type: http
  runner: auto
  config:
    url: https://api.example.com/health
    method: GET
    expectedStatus: [200]
    followRedirects: false
    verifyTLS: true
    maxBodyBytes: 16384
```

| `config.*` | Тип | Обов. | Default | Семантика / драйвер |
|---|---|---|---|---|
| `url` | URL | Так | — | Абсолютний `http(s)` URL без userinfo/fragment. |
| `method` | enum `GET`,`HEAD` | Ні | `GET` | Дозволені read-only методи в v1alpha1; `POST`, `PUT`, `DELETE` заборонені. |
| `expectedStatus` | array<integer 100…599> | Ні | `[200]` | Допустимі HTTP-коди; порожній масив заборонено. |
| `followRedirects` | boolean | Ні | `false` | Кожен перехід перевіряється agent target policy, не тільки початковий URL. |
| `verifyTLS` | boolean | Ні | `true` | Значення `false` **заборонене** в v1alpha1; зокрема для обходу DNS. |
| `headers` | map<string,string> | Ні | `{}` | Лише несекретні HTTP-заголовки; `Authorization`, `Cookie`, `Proxy-Authorization` і CRLF заборонені в YAML. |
| `bodyMatchesRegex` | regex | Ні | — | Умова на вміст body, **без** повернення груп захоплення; підтримується `blackbox` та `native-http`. |
| `maxBodyBytes` | integer 1…65536 | Ні | `16384` | Верхня межа обробки body, не дозвіл на архівування; для `native-http` застосовується безпосередньо; Blackbox — через підтримуваний ліміт модуля. |
| `connectIP` | IP literal | Ні | — | **Лише** `native-http`: встановити TCP-з'єднання до цієї IP-адреси, зберігаючи hostname URL для Host, SNI та TLS certificate verification. Це інструмент DNS-діагностики. |

`headers` не повинні містити токенів; авторизовані endpoints/credentials виходять за межі базового v1alpha1. `connectIP` та `extract` змушують `runner: auto` обрати `native-http`; якщо явно зазначено `runner: blackbox` із цими полями — помилка валідації. В один запуск — **один** HTTP-запит одним runner, без прихованих паралельних дублів.

### 5.5. Визначення runner

| Runner | `icmp` | `tcp` | `dns` | `http` | Призначення |
|---|---|---|---|---|---|
| `blackbox` | Так | Так | Так | Так | Виклик локального Blackbox Exporter з попередньо сконфігурованими / перевіреними modules. |
| `native-http` | Ні | Ні | Ні | Так | Обмежена HTTP-перевірка на Go з доступом до bounded body/extraction та `connectIP`. |
| `auto` | Вибирає blackbox | Вибирає blackbox | Вибирає blackbox | Blackbox за замовч., native за `extract`/`connectIP` | Статичний вибір під час компіляції; не змінюється мовчки під час виконання. |

**Важливо:** можливості Blackbox залежать від **закріпленої версії образу й конфігурації module**. Драйвер v1alpha1 не повинен пропускати в YAML довільні параметри зовнішнього exporter; він переводить підтримувані поля NetMonOrc у валідувану конфігурацію runner. `http.expectedStatus` і `http.bodyMatchesRegex` не означають отримання повного body через API exporter.

---
## 6. `ExtractSpec` — отримання типізованих значень із відповіді

`extract` — вкладений масив усередині `CheckSpec` або `DiagnosticAction.check`; не є окремим ресурсом. У v1alpha1 він реалізується лише для HTTP (`native-http`) та **не** означає централізований збір логів.

```yaml
extract:
  - name: queueDepth
    from: response.body
    using: regex
    pattern: 'queue_depth=(?P<value>[0-9]+)'
    group: value
    as: integer
    onMissing: unknown
```

| Поле `extract[]` | Тип | Обов. | Default | Роль / перевірки |
|---|---|---|---|---|
| `name` | CheckID | Так | — | Ім'я типізованого результату, напр. `values.queueDepth`; унікальне в check. |
| `from` | enum `response.body` | Так | — | Джерело, яке дозволено читати у v1alpha1. |
| `using` | enum `regex` | Ні | `regex` | Вибір extractor; реєстр розширюється новими зареєстрованими `using`. |
| `pattern` | Regex | Так | — | RE2-сумісний шаблон, валідований при `apply`; довжина до 2048 символів. |
| `group` | string/positive integer | Так | — | Іменована або номерна capture group. Група має існувати у `pattern`. |
| `as` | enum `string`,`integer`,`float`,`boolean` | Ні | `string` | Цільовий тип; провал перетворення → `extractionStatus: error`. |
| `onMissing` | enum `unknown`,`error` | Ні | `unknown` | `unknown` означає відсутність значення без підстановки нуля. |

**Результат:** `Observation.values[name]` із окремим `extractionStatus`. `onMissing` **не** змінює успішність HTTP status check; якщо відсутність поля сама є порушенням сервісу, це потрібно явно описати CEL-правилом. Raw HTTP body за замовчуванням **не зберігається** у PostgreSQL; тільки обмежено зчитується під час execution.

**Розширення:** майбутній `using: json-field` може мати власний `path` і validator без зміни YAML Decoder. Поки значення `json-field` у v1alpha1 є помилкою.

---

## 7. `ExecutionFallback` — оброблення **технічної** неможливості виконання

```yaml
fallback:
  enabled: true
  on: [agent_unreachable, runner_unavailable]
  strategy: same-zone
  maxAttempts: 1
```

| Поле `fallback.*` | Тип | Обов. | Default | Призначення |
|---|---|---|---|---|
| `enabled` | boolean | Ні | `false` | Якщо `false`, решта полів не допускаються (крім самого `enabled`). |
| `on` | array<enum> | Так, коли enabled | — | Дозволено `agent_unreachable`, `runner_unavailable`, `local_execution_error`, `config_rejected`. |
| `strategy` | enum `same-zone`,`different-zone` | Ні | `same-zone` | `same-zone` прагне відновити покриття тієї самої зони; `different-zone` створює **іншу** перспективу, не зараховується як покриття первинної зони. |
| `maxAttempts` | integer 1…2 | Ні | `1` | Максимум додаткових спроб (понад початкову). |

**Заборонено:** запуск fallback лише через `http_503`, `dns_nxdomain`, `tcp_target_timeout` або невдалий `bodyMatchesRegex`; це не доводить поломки виконавця. Негативні результати цілі вимагають `DiagnosticProfile`, не `ExecutionFallback`. Кожна спроба має свій `Observation`/`taskAttempt`, спільний кореляційний ключ і зберігає оригінал.

**Часткова мережева партиція:** якщо старий агент продовжує працювати за останнім застосованим планом, Controller не може гарантувати, що його виконання вже зупинилося. Fallback обмежується лімітами та відображає можливі дублікати за ідентичністю запуску, а не обіцяє exactly-once execution.

---

## 8. `PlacementPolicy` та `PlacementSpec` — де виконувати checks

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: PlacementPolicy
metadata:
  name: dual-region
spec:
  selector:
    matchLabels: {role: probe}
  zones: [eu-west, us-east]
  replicas: 2
  distinctBy: zone
  strategy: leastLoaded
  onInsufficientAgents: pending
```

**`PlacementSpec`** — поля всередині `PlacementPolicy.spec`, `Monitor.placement`, `Check.placement`, `DiagnosticAction.placement`. Ключі та значення скрізь однакові.

| Поле | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `agentRefs` | array<registered Agent name> | Ні | `[]` | Явні допустимі агенти; беруться лише зареєстровані й авторизовані. За наявності — перетин з іншими фільтрами. |
| `selector` | `LabelSelector` | Ні | `{}` | Вибір агента за **довіреними Controller-managed** labels. |
| `zones` | array<string> | Ні | `[]` | Допустимі **зони спостереження**; порожній список — всі дозволені зони. |
| `replicas` | integer 1…10 | Ні | `1` | Кількість **одночасних точок** для кожного scheduled check. |
| `distinctBy` | enum `none`,`zone`,`failureDomain` | Ні | `none` | Вимога різних зон або встановлених failure domains; географія ≠ гарантована незалежність маршруту. |
| `strategy` | enum `leastLoaded`,`roundRobin`,`fixed` | Ні | `leastLoaded` | Спосіб вибору після фільтрації capabilities, локальної політики та зон. `fixed` вимагає `agentRefs` із достатньою кількістю агентів. |
| `onInsufficientAgents` | enum `pending`,`partial` | Ні | `pending` | `pending`: не вдавати виконання повного набору replicas; `partial`: використовувати доступну частину й позначити `coverage_degraded`. |

### 8.1. `LabelSelector`

| Поле | Тип | Default | Семантика |
|---|---|---|---|
| `matchLabels` | map<string,string> | `{}` | Всі пари мають збігтися (`AND`). |
| `matchExpressions` | array<object> | `[]` | Умови `key`, `operator`, `values`; усі вирази поєднуються через `AND`. |
| `matchExpressions[].key` | string | — | Назва label. |
| `matchExpressions[].operator` | enum `In`,`NotIn`,`Exists`,`DoesNotExist` | — | Семантика відповідно до назви. |
| `matchExpressions[].values` | array<string> | `[]` | Для `In/NotIn` непорожній, для `Exists/DoesNotExist` має бути порожнім. |

**Кандидат допустимий**, якщо він зареєстрований, online за політикою, підтримує `type`/`runner`, йому дозволено перевірити фактичні адреси, відповідає selector/zones і вміщується в локальні ліміти. `strategy` не може обійти заборони. Якщо Controller не знає точної доступності цілі з агента, він не обіцяє її; це підтверджує фактичний результат.

**Зв'язок із матрицями §21:** можливості агентів `C`, призначення `X`, адміністративні зони `Z`. `replicas` — планова кількість, а не гарантія отриманих спостережень.

---

## 9. `AssessmentPolicy` та `AssessmentSpec` — як оцінювати результати

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: AssessmentPolicy
metadata:
  name: standard-web-assessment
spec:
  requiredZones: [eu-west, us-east]
  zoneAggregation: all-required
  freshness:
    maxAge: 90s
    maxSkew: 25s
  minCoverage: 0.80
  checks:
    - {id: icmp, weight: 0.10, required: false}
    - {id: tcp443, weight: 0.30, required: false}
    - {id: https, weight: 0.60, required: true}
  rules:
    - id: http-failed
      priority: 100
      when: 'checks["https"].outcome == "failure"'
      then: {state: down, reason: HTTP_CHECK_FAILED}
    - id: tcp-failed
      priority: 80
      when: 'checks["tcp443"].outcome == "failure"'
      then: {state: degraded, reason: TCP_CHECK_FAILED}
  weighted:
    enabled: true
    healthyMin: 0.90
    degradedMin: 0.50
  defaultState: unknown
```

### 9.1. Поля `AssessmentSpec`

| Поле | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `requiredZones` | array<string> | Ні | Зони застосованих призначень | Зони, з яких **потрібні** спостереження. Для багатоточкового SLA бажано задавати явно; відсутня зона → coverage degraded. |
| `zoneAggregation` | enum `all-required`,`any` | Ні | `all-required` | Як переходити від `ZoneAssessment` до Monitor. `any` дозволений лише за явною політикою, коли доступність із будь-якої однієї зони достатня. |
| `sameZoneAggregation` | enum `all`,`any` | Ні | `all` | Якщо один Check має кілька агентів у тій самій зоні, визначає агреговане `checks["id"].outcome`; окремі Observations залишаються доступними. |
| `freshness` | object | Ні | Див. нижче | Умови придатності за часом/ревізією. |
| `minCoverage` | Weight 0…1 | Ні | `1.0` | Мінімальна зважена повнота для твердження `healthy`; не замінює обов'язкові checks. |
| `checks` | array<`AssessmentCheck`> | Ні | Рівні ваги для enabled checks; всі `required: false` | Ваги й обов'язковість checks у **цьому Monitor**. Кожний check ID повинен існувати після профільного expansion. |
| `rules` | array<`AssessmentRule`> | Ні | `[]` | CEL-предикати пріоритетного визначення зонального стану. |
| `weighted` | `WeightedPolicy` | Ні | `enabled: true`, пороги §9.4 | Резервна оцінка для випадків без спрацьованого правила. |
| `defaultState` | enum `unknown`,`degraded`,`down` | Ні | `unknown` | Результат за відсутності достатньої інформації; `healthy` заборонено як fallback. |

### 9.2. `freshness`

| Поле | Тип | Обов. | Default | Правило |
|---|---|---|---|---|
| `maxAge` | Duration | Ні | `max(3 × Check.interval, Check.interval + 2 × Check.timeout)` для кожного check | Результат старіший за TTL не враховується як поточний. Якщо задано явно, має бути ≥ найбільшого `interval` залучених checks. |
| `maxSkew` | Duration | Ні | `30s` | Максимальна різниця часу між результатами, що одночасно використовуються у cross-check правилі; за перевищення — контекст позначається неповним. |

Стан поточного Monitor обчислюється лише за результатами **поточної застосованої ревізії** відповідного агента. Старі ревізії та delayed replay залишаються в історії, але не змінюють live-state заднім числом. Виміряна тривалість — за монотонним годинником, timestamp — UTC; при суттєвому clock skew дані не слід вважати підтверджено актуальними.

### 9.3. `AssessmentCheck`

| Поле | Тип | Обов. | Default | Значення |
|---|---|---|---|---|
| `id` | CheckID | Так | — | ID check після розгортання `useProfiles`. |
| `weight` | Weight | Ні | 1 до нормування | **Вага результату**, окремо від `CheckSpec.priority`. Ваги нормуються на суму для застосовних checks. |
| `required` | boolean | Ні | `false` | Без актуального успіху цього check **не можна** стверджувати `healthy` у зоні. Негативний результат не компенсується вагами інших checks. |

За відсутності `checks` політика призначає рівні ненульові ваги **всім enabled checks**. Якщо список `checks` явно задано, він повинен містити **кожний enabled check рівно один раз** (це виключає неявне випадіння перевірки з формули). Не допускається сума ваг 0. Перед обчисленням використовується нормування до суми 1.

### 9.4. `WeightedPolicy`

| Поле | Тип | Default | Вимога |
|---|---|---|---|
| `enabled` | bool | `true` | Якщо `false`, за відсутності matched `rules` використовується `defaultState`. |
| `healthyMin` | Weight | `0.90` | Поріг успішності, щоб отримати `healthy`, якщо coverage і required checks дозволяють. |
| `degradedMin` | Weight | `0.50` | За нижчого score та достатності доказів можливий `down`. `0 ≤ degradedMin < healthyMin ≤ 1`. |

Визначення метрик: `H = Σ(w·v·h)/Σ(w·v)` — зважена частка успішних **наявних** перевірок; `Q = Σ(w·v)/Σw` — зважене покриття. `h` = 1 для підтвердженого success, 0 для підтвердженого failure, `v` = 1 лише за придатного Observation. За нульового знаменника `H` не визначається. Це **похідні оцінки**, а не ймовірності надійності.

### 9.5. `AssessmentRule`

| Поле | Тип | Обов. | Default | Значення |
|---|---|---|---|---|
| `id` | CheckID | Так | — | Ідентифікатор правила. |
| `priority` | integer 1…1000 | Ні | `100` | Більше — вище. При однаковому пріоритеті — лексикографічно за `id`. |
| `when` | CELExpression → bool | Так | — | Умова над контекстом **однієї зони** (або окремого агрегованого Monitor за явно відведеним API). |
| `then.state` | enum `healthy`,`degraded`,`down`,`unknown` | Так | — | Стан за умови match; чинні інваріанти required/coverage не допускають необґрунтованого healthy. |
| `then.reason` | code `[A-Z0-9_]+` | Так | — | Пояснюваний машинний reason code. |

**Порядок:** відсортувати `rules` за `priority DESC`, далі `id ASC`; **перше** істинне правило визначає кандидатний стан. `false` — наступне правило; `unknown`/`evaluation error` — не трактувати як `false` для дозволу `healthy`, зафіксувати причину неповного оцінювання. Потім застосувати інваріанти: `healthy` неможливий за `Q < minCoverage`, відсутності/відмови required check або неузгоджених ревізій. Підтверджений `failure` може дати `degraded/down` навіть за часткового coverage, але це відображається окремим `coverage_degraded`.

**Зональний результат та глобальна політика:** при `all-required` недоступність будь-якої обов'язкової зони не може бути прихована успіхом інших зон; агрегований стан `degraded/down` відповідно до правил і втрат. При відсутності даних — `unknown` для цієї зони + `coverage_degraded` для Monitor. `any` не означає арифметичне голосування й потребує прямого рішення адміністратора.

### 9.6. Композитний стан і coverage — різні поля

| Поле runtime | Можливі значення | Коли встановлюється |
|---|---|---|
| `ZoneAssessment.state` | `healthy`,`degraded`,`down`,`unknown` | На основі застосовних check outcomes у конкретній зоні. |
| `MonitorAssessment.state` | ті самі | На основі `zoneAggregation`. |
| `coverage.state` | `sufficient`,`degraded`,`unknown` | Наявність актуальних перевірок потрібних зон/required checks. |
| `assessment.reasonCodes` | array<string> | Пояснення правила / інваріантів, напр. `REQUIRED_CHECK_FAILED`, `STALE_OBSERVATIONS`. |
| `assessment.evidenceIDs` | array<ObservationID> | Посилання на конкретні докази. |

### 9.7. Кілька агентів в одній зоні: явна політика

| `sameZoneAggregation` | Усі актуальні results success | Є success і failure | Усі results failure | Немає актуальних results |
|---|---|---|---|---|
| `all` (default) | `success` | `failure` + reason `AGENT_RESULTS_DIVERGE` | `failure` | `unknown` + coverage degraded |
| `any` (лише явне рішення) | `success` | `success` + reason `AGENT_RESULTS_DIVERGE`; первинна failure зберігається | `failure` | `unknown` + coverage degraded |

Для `all` відсутність одного з **призначених** виконавців при наявному успіху інших дає успіх **серед отриманих**, але `coverage_degraded`; повне `healthy` не підтверджується, якщо бракує required coverage. `any` слід застосовувати тільки якщо дійсно достатньо одного доступного шляху. Навіть тоді всі розбіжності відображаються у вихідних доказах.

`required` застосовується до зонального стану check з урахуванням фактичного покриття. Декілька агентів однієї зони не обов'язково є незалежними точками: для цього слід використовувати `distinctBy: failureDomain` і враховувати реальну мережеву топологію.

---
## 10. `DiagnosticBinding` — зв'язування Monitor і профілю діагностики

Діагностика запускається лише після оцінювання та за визначеним тригером. Наявність профілю не запускає перевірок сама по собі.

```yaml
diagnostics:
  enabled: true
  profileRef: web-troubleshooting
  onStates: [degraded, down]
  when: 'monitor.coverageState != "unknown"'
  cooldown: 5m
```

| Поле | Тип | Обов. | Default | Правило |
|---|---|---|---|---|
| `enabled` | boolean | Ні | `false` | У вимкненому стані додаткові diagnostics не створюються. |
| `profileRef` | Ref → `DiagnosticProfile` | Умов. | — | Обов'язковий при `enabled: true`. |
| `onStates` | array<`degraded`,`down`,`unknown`> | Ні | `[degraded,down]` | Допустимі стани для trigger. `healthy` не підтримується як автоматичний trigger. |
| `when` | CEL → bool | Ні | `true` | Додаткова умова в **monitor-level** контексті. |
| `cooldown` | Duration | Ні | `5m` | Мінімальний інтервал між новими сесіями одного типу після завершення попередньої. |

**Інваріант:** не більше однієї активної `DiagnosticSession` на `Monitor` для поточного інцидентного епізоду. Перезапуск при оновленні Observation не скидає бюджети попереднього епізоду. Втрата coverage сама по собі не породжує необмежені діагностичні DAG.

---

## 11. `DiagnosticProfile` — fixed/adaptive додаткова діагностика

**Сенс:** описує **дозволені** дії, умови і бюджети. У `fixed` користувач задає DAG, у `adaptive` Planner обирає найбільш розрізнювальну наступну дію з того самого каталогу. Обидва режими використовують той самий runner/agent model.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: DiagnosticProfile
metadata:
  name: web-troubleshooting
spec:
  mode: adaptive
  budget:
    maxAdditionalChecks: 5
    maxDuration: 60s
    maxParallel: 2
  hypotheses:
    - {id: dns-failure, description: Problem with name resolution}
    - {id: service-failure, description: HTTP application failure}
    - {id: zone-path-failure, description: Path-specific connectivity problem}
  actions:
    - id: dns-resolver-a
      check:
        type: dns
        timeout: 4s
        config:
          resolver: 192.0.2.53:53
          queryName: api.example.com
          recordType: A
      expectedOutcomes:
        dns-failure: failure
        service-failure: success
        zone-path-failure: success
      costUnits: 1
    - id: http-other-zone
      check:
        type: http
        timeout: 5s
        config: {url: 'https://api.example.com/health', expectedStatus: [200]}
      placement:
        selector: {matchLabels: {role: probe}}
        zones: [us-east]
        replicas: 1
      expectedOutcomes:
        dns-failure: either
        service-failure: failure
        zone-path-failure: success
      costUnits: 1
  adaptive:
    algorithm: distinguish-pairs-per-cost
    stopWhenUnique: true
    maxNoGainSteps: 1
```

### 11.1. `DiagnosticProfile.spec`

| Поле | Тип | Обов. | Default | Роль / обмеження |
|---|---|---|---|---|
| `mode` | enum `fixed`,`adaptive` | Так | — | Варіант виконання. |
| `budget` | `DiagnosticBudget` | Так | — | Жорсткі межі запусків, часу, паралельності. |
| `hypotheses` | array<`FaultHypothesis`> | Умов. | `[]` | Для `adaptive` — мінімум 2; для `fixed` необов'язкові. **Класи спостережуваних несправностей**, не гарантовані фізичні причини. |
| `actions` | array<`DiagnosticAction`> | Так | — | Каталог допустимих одноразових перевірок; не менше однієї. |
| `fixed` | `FixedDAG` | Умов. | — | Обов'язковий лише при `mode: fixed`; для adaptive заборонений. |
| `adaptive` | `AdaptivePolicy` | Умов. | — | Обов'язковий лише при `mode: adaptive`; для fixed заборонений. |

### 11.2. `DiagnosticBudget`

| Поле | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `maxAdditionalChecks` | integer 1…50 | Так | — | Сумарна кількість фізичних додаткових **спроб**, включно з retries/fallback. Не покриває базові регулярні checks. |
| `maxDuration` | Duration 1s…10m | Так | — | Загальний deadline сесії з моменту trigger. |
| `maxParallel` | integer 1…10 | Ні | `1` | Максимум одночасних додаткових checks цього профілю. |
| `maxEstimatedBytes` | integer >0 | Ні | — | Опційний **оціночний** бюджет трафіку; не обіцяє точного підрахунку байтів на IP/Wire. Якщо заданий — кандидати, що очевидно перевищують залишок, відхиляються. |

**Захист від штормів:** глобальні бюджети Controller, ліміти цілі та агента мають пріоритет над локальним `budget`. За спрацювання загального ліміту сесія переходить у `inconclusive` із кодом `BUDGET_EXHAUSTED`.

### 11.3. `FaultHypothesis`

| Поле | Тип | Обов. | Default | Сенс |
|---|---|---|---|---|
| `id` | CheckID | Так | — | Унікальний ідентифікатор класу несправності. |
| `description` | string | Ні | `""` | Пояснення для людини. |

Гіпотези в v1alpha1 — **не ймовірнісні**. Без калібрування даних NetMonOrc не відображає «ймовірність 97%». Можливий підсумок — один підтриманий клас, перелік сумісних класів або `inconclusive`.

### 11.4. `DiagnosticAction`

| Поле | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `id` | CheckID | Так | — | Ідентифікатор кандидата для DAG/адаптивного селектора. |
| `check` | `OneShotCheck` | Так | — | Одноразовий check: як `CheckSpec`, але **без** `id`, `interval`, `jitter`, `priority`, `enabled`, `placement` і `fallback`; містить `type`, `runner`, `timeout`, `config`, `extract`. |
| `placementRef` | Ref → `PlacementPolicy` | Ні | — | Правила точки спостереження саме для кандидата; oneOf із inline `placement`. |
| `placement` | `PlacementSpec` | Ні | — | Якщо відсутній — policy Monitor; для дослідження різних зон задавати явно. |
| `expectedOutcomes` | map<HypothesisID, OutcomePattern> | Умов. | `{}` | Для `adaptive` обов'язково покрити **кожну** гіпотезу. Для fixed не обов'язково. |
| `costUnits` | number >0 | Ні | `1` | Нормована відносна ціна action для алгоритму; фактичні checks завжди обліковуються окремо. |

`OutcomePattern` = `success`, `failure`, `unknown`, `either`. `either` означає, що дія **не розрізняє** відповідну гіпотезу (можливі всі результати); `unknown` означає, що очікувано немає придатного спостереження. Відмінність `failure` і `execution error` принципова; технічна помилка не вважається підтвердженням гіпотези про сервіс.

**Для одного action з `placement.replicas > 1`:** кожен реально виконаний probe враховується в `maxAdditionalChecks` окремо. Для простоти v1alpha1 рекомендує `replicas: 1` у діагностиці; одночасні різнозональні перевірки описують окремими `DiagnosticAction`.

### 11.5. `FixedDAG`

```yaml
fixed:
  steps:
    - id: verify-tcp
      actionRef: tcp-other-zone
      when: 'monitor.state != "healthy"'
      dependsOn: []
    - id: verify-dns
      actionRef: dns-resolver-a
      dependsOn: [verify-tcp]
      when: 'tasks["verify-tcp"].outcome != "success"'
```

| Поле | Тип | Обов. | Default | Сенс |
|---|---|---|---|---|
| `fixed.steps` | array<`FixedStep`> | Так для fixed | — | Не менше одного елемента. |
| `steps[].id` | CheckID | Так | — | Унікальний ID вузла DAG. |
| `steps[].actionRef` | Ref → `actions[].id` | Так | — | Яку дію виконати. |
| `steps[].dependsOn` | array<StepID> | Ні | `[]` | Дії попередників, які **завершилися**; відсутній ID/цикл — помилка. |
| `steps[].when` | CEL → bool | Ні | `true` | Умова після завершення всіх `dependsOn`; `unknown/error` → step не виконується, діагностика фіксує недостатність контексту. |

DAG виконується з топологічним порядком, паралельність обмежується `maxParallel`; ребра не утворюють безмежних циклів. `else` описується наступним правилом із протилежним `when` або гілкою за замовчуванням — окремий виконуваний `else:` у v1alpha1 **відсутній**. Один і той самий `actionRef` може бути включений у декілька кроків, але кожен фізичний запуск обліковується.

### 11.6. `AdaptivePolicy`

| Поле | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `algorithm` | enum `distinguish-pairs-per-cost` | Ні | `distinguish-pairs-per-cost` | Критерій: скільки ще сумісних пар гіпотез очікувано розділяє дія, поділено на `costUnits`. |
| `stopWhenUnique` | boolean | Ні | `true` | Зупинятися при одному обґрунтовано сумісному класі (за наявності достатніх/актуальних evidence). |
| `maxNoGainSteps` | integer 0…5 | Ні | `0` | Допустиме число безрезультатних діагностичних кроків перед `inconclusive`. |
| `tieBreak` | enum `actionIdAsc` | Ні | `actionIdAsc` | Детермінований вибір серед рівноцінних кандидатів. |

Селектор формує список допустимих пар `action × agent`, відкидає несумісні capabilities/targets/зони/бюджети, оцінює потенційну розрізнювальність із `expectedOutcomes` і вибирає кандидат. Результат оновлює набір сумісних класів; якщо спостереження шумні або суперечливі — повертаються кілька гіпотез/`inconclusive`, а не вигадана точна причина. Незалежність перевірок оцінюється окремо за `failureDomain`; дві точки з тим самим шляхом не слід прирівнювати до незалежних доказів.

---

## 12. `CheckProfile` — повторно використовувані перевірки

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: CheckProfile
metadata:
  name: standard-https
spec:
  parameters:
    - {name: url, type: url, required: true}
    - {name: port, type: integer, required: false, default: 443}
  checks:
    - id: tcp
      type: tcp
      config: {port: '${params.port}'}
    - id: https
      type: http
      config:
        url: '${params.url}'
        expectedStatus: [200]
```

### 12.1. `CheckProfile.spec`

| Поле | Тип | Обов. | Default | Сенс |
|---|---|---|---|---|
| `parameters` | array<`ProfileParameter`> | Ні | `[]` | Формальні параметри з типами/значеннями за замовчуванням. |
| `checks` | array<`CheckSpec` із параметрами> | Так | — | Непорожній список. Усі ID унікальні в профілі. |
| `description` | string ≤1024 | Ні | `""` | Опис сценарію повторного використання. |

### 12.2. `ProfileParameter`

| Поле | Тип | Обов. | Default | Значення |
|---|---|---|---|---|
| `name` | CheckID | Так | — | Назва, що використовується в `${params.<name>}`. |
| `type` | enum `string`,`integer`,`float`,`boolean`,`url`,`ip`,`hostname` | Так | — | Типізована перевірка після підстановки. |
| `required` | boolean | Ні | `false` | Якщо `true`, `default` заборонений і значення має бути передане. |
| `default` | відповідний scalar | Ні | — | Значення за замовчуванням, якщо параметр необов'язковий. |
| `description` | string | Ні | `""` | Людинозрозумілий коментар. |

**Розширення:** профіль не може містити `useProfiles` (немає рекурсивного вкладення), YAML merge `<<` чи довільний генератор полів. Компілятор підставляє параметри, префіксує ID `as__id`, повністю валідує згенерований `CheckSpec`, після чого формує Monitoring IR. Якщо `AssessmentPolicy` посилається на профільний check, вона повинна вказувати вже розгорнутий ID на кшталт `web__https`.

---
## 13. `AlertRule` — умови формування й завершення алертів

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: AlertRule
metadata:
  name: api-unavailable
spec:
  monitorRef: corporate-api
  scope: monitor
  when: 'monitor.state == "down"'
  resolveWhen: 'monitor.state == "healthy"'
  severity: critical
  for: 60s
  cooldown: 5m
  channelRef: default-webhook
  notifyEvents: [firing, resolved]
  enabled: true
```

| Шлях `spec.*` | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `monitorRef` | Ref → `Monitor` | Умов. | — | Один конкретний Monitor; **oneOf** із `selector`. |
| `selector` | `LabelSelector` | Умов. | — | Відбір Monitor за `metadata.labels`. Не обробляє labels Target без явного посилання. |
| `scope` | enum `monitor`,`zone` | Ні | `monitor` | `monitor`: одна група alert events на Monitor. `zone`: окремий alert instance для кожної зони. |
| `when` | CEL → bool | Так | — | Умова переходу в pending/firing; використовує агрегований або зональний контекст згідно `scope`. |
| `resolveWhen` | CEL → bool | Ні | `'monitor.state == "healthy"'` | Явна умова завершення; за `unknown` алерт не розв'язується автоматично. Для `scope: zone` default — `'zone.state == "healthy"'`. |
| `severity` | enum `info`,`warning`,`critical` | Ні | `warning` | Рівень важливості; сам по собі не змінює стан Monitor. |
| `for` | Duration ≥0 | Ні | `0s` | Мінімальна тривалість безперервного виконання `when` перед `firing`. |
| `cooldown` | Duration ≥0 | Ні | `5m` | Захист від повторних однакових повідомлень. Не перешкоджає збереженню фактичних змін стану. |
| `channelRef` | Ref → `WebhookEndpoint` | Ні | `default-webhook` | Єдиний глобальний канал; має існувати при `enabled: true`. |
| `notifyEvents` | array<`firing`,`resolved`> | Ні | `[firing,resolved]` | Події, які виходять у webhook. Дублі не допускаються. |
| `enabled` | boolean | Ні | `true` | Вимикає створення нових спрацювань за правилом. |
| `description` | string | Ні | `""` | Опис алерта. |

**Життєвий цикл:** `normal → pending → firing → resolved`. Якщо умова перестала виконуватися до часу `for`, pending скидається. Якщо результат `unknown`, це не доказ ні firing, ні resolved; відповідне правило не повинно непомітно завершувати критичний алерт. Унікальний ключ dedup включає `AlertRule`, `Monitor`, `scope` і, для зонального алерта, `zone`.

**Повідомлення та зберігання:** Alert Manager створює подію; Webhook Sender бере її з durable PostgreSQL outbox. Доставка — з можливими повторами, одержувач дедуплікує за `event_id`. В v1alpha1 не існує `telegram:`, `email:`, `slack:` та інших notification kinds.

---

## 14. `WebhookEndpoint` — **єдиний** вихідний канал алертів

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: WebhookEndpoint
metadata:
  name: default-webhook
spec:
  url: https://alerts-gateway.example.net/netmonorc
  enabled: true
  auth:
    type: bearer
    secretRef: alert-webhook-token
  timeout: 5s
  retry:
    maxAttempts: 5
    initialBackoff: 2s
    maxBackoff: 1m
```

| Шлях `spec.*` | Тип | Обов. | Default | Значення |
|---|---|---|---|---|
| `url` | HTTPS URL | Так | — | Кінцева адреса; тільки `https://`, без userinfo та fragment. Адреса повинна бути дозволена webhook egress policy. |
| `enabled` | boolean | Ні | `true` | Один активний екземпляр у межах інсталяції. |
| `auth` | object | Ні | `{type:none}` | Вибір захисту каналу; не зберігати токен у маніфесті. |
| `auth.type` | enum `none`,`bearer` | Ні | `none` | Для bearer — заголовок `Authorization: Bearer ...`. |
| `auth.secretRef` | string | Умов. | — | Ім'я **попередньо встановленого секрету Master Stack**; потрібне для `bearer`, за `none` заборонене. |
| `timeout` | Duration 1s…30s | Ні | `5s` | Час одного HTTP POST. |
| `retry` | object | Ні | Див. нижче | Політика повторних спроб. |
| `retry.maxAttempts` | integer 1…10 | Ні | `5` | Загальна кількість спроб, включно з першою. |
| `retry.initialBackoff` | Duration 1s…5m | Ні | `2s` | Початкова затримка перед retry. |
| `retry.maxBackoff` | Duration 1s…30m | Ні | `1m` | Верхня межа exponential backoff із jitter. |

**Інваріанти:** не більше одного `WebhookEndpoint` із `enabled: true`. Дозволено створити/валідувати неактивний ресурс для заміни, але `AlertRule.channelRef` на нього не доставлятиме події. За типовим налаштуванням є ресурс `default-webhook`, якщо він створений адміністратором (не створюється з секретом автоматично).

### 14.1. Стандартний payload webhook (створюється системою, не YAML)

| JSON-поле | Тип | Пояснення |
|---|---|---|
| `event_id` | UUID | Стабільний ID для дедуплікації повторних HTTP POST. |
| `event_type` | `alert.firing` / `alert.resolved` | Фактичний тип переходу. |
| `alert_rule` | string | `metadata.name` правила. |
| `monitor` | string | Monitor, до якого належить alert. |
| `zone` | string/null | Зона для `scope: zone`. |
| `state` | string | Актуальний стан ресурсу на момент події. |
| `severity` | enum | Рівень сповіщення. |
| `reason_codes` | array<string> | Чому сформувався стан. |
| `evidence_ids` | array<string> | Посилання на Observations. |
| `occurred_at` | RFC3339 UTC | Час переходу алерта. |
| `schema_version` | string | Версія структури webhook (`1`). |

Webhook не включає raw HTTP body, приватні credentials чи адміністраторські паролі. Редиректи вимкнені; фактичні IP-адреси перевіряються за egress policy, щоб не створити SSRF-канал.

---

## 15. `Dependency` — граф залежностей між ресурсами

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Dependency
metadata:
  name: portal-requires-dns
spec:
  from: {kind: Target, name: corporate-portal}
  to: {kind: Target, name: corporate-dns}
  relation: requires
  strength: hard
  description: Вебсервіс використовує корпоративний resolver
```

| Поле `spec.*` | Тип | Обов. | Default | Значення |
|---|---|---|---|---|
| `from` | `ResourceReference` | Так | — | Об'єкт, який **залежить** від іншого. |
| `to` | `ResourceReference` | Так | — | Об'єкт залежності. |
| `relation` | enum `requires`,`routesThrough`,`hostedOn`,`resolvesVia`,`relatedTo` | Ні | `requires` | Семантична природа зв'язку. |
| `strength` | enum `hard`,`soft` | Ні | `soft` | Експертна позначка важливості залежності; сама по собі не переносить стан/помилку. |
| `description` | string ≤1024 | Ні | `""` | Пояснення зв'язку. |

### 15.1. `ResourceReference`

| Поле | Тип | Обов. | Правило |
|---|---|---|---|
| `kind` | enum `Target`,`Monitor` | Так | Вказує клас об'єкта. |
| `name` | Ref | Так | Має існувати на момент застосування. |

**Напрям:** `from → to` означає `from` **залежить від** `to`; `D[from,to] = 1`. Самопосилання заборонені. Цикли `requires` не допускаються при побудові похідного DAG; для інших типів відношень можливі цикли, але вони **не** означають циклічну рекурсію стану. `Dependency` використовується як контекст діагностики, а не доказ causality. Короткий запис `Monitor.spec.dependencies` трактує залежності як `requires` цього Monitor до перелічених `Target`.

---

## 16. `ResourceGroup` — логічні/пасивні та похідні ресурси

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: ResourceGroup
metadata:
  name: production-web
spec:
  selector:
    matchLabels: {environment: production, service: web}
  mode: derived
  evaluation:
    strategy: all-required
    minCoverage: 1.0
```

| Шлях `spec.*` | Тип | Обов. | Default | Мета |
|---|---|---|---|---|
| `selector` | `LabelSelector` | Умов. | — | Застосовується до **`Monitor.metadata.labels`**. OneOf із `members`. |
| `members` | array<Ref → `Monitor`> | Умов. | — | Фіксований склад групи; oneOf із `selector`. |
| `mode` | enum `context`,`derived` | Ні | `context` | `context`: група для фільтрації, **без власного health**. `derived`: стан обчислюється на основі станів Monitor. |
| `evaluation` | `GroupEvaluation` | Умов. | — | Дозволений/обов'язковий тільки при `mode: derived`. |
| `description` | string | Ні | `""` | Людинозрозуміле призначення групи. |

### 16.1. `GroupEvaluation`

| Поле | Тип | Обов. | Default | Семантика |
|---|---|---|---|---|
| `strategy` | enum `all-required`,`any`,`weighted` | Так | — | `all-required`: кожен Monitor необхідний; `any`: достатньо одного доступного; `weighted`: зважений агрегат із чітким порогом. |
| `minCoverage` | Weight | Ні | `1.0` | Мінімальна частка актуальних станів Monitor для `healthy`. |
| `weights` | map<MonitorName,Weight> | Умов. | — | Лише `weighted`, лише при `members` (фіксований список); сума >0, нормування до 1. |
| `healthyMin` | Weight | Умов. | `0.9` | Лише `weighted`, поріг статусу `healthy`; за нижчого score результат щонайменше `degraded`, а за недостатності покриття — `unknown`. |

**Особливості:** похідна група не виконує власні probes, не дублює Observations і **не** може бути учасником іншої групи в v1alpha1; це запобігає циклічному переобчисленню. `context`-група може мати порожній список членів без health; `derived` із нулем членів має стан `unknown` і `coverage: unknown`. Зміна labels Monitor змінює склад селекторної групи після reconciliation.

**Ваги групи** не переносяться на `AssessmentPolicy` окремих Monitor. `GroupEvaluation` — окремий рівень оцінювання та окрема функція, а не середнє від сирих ICMP/HTTP-результатів.

---

## 17. `Component` — інтеграція з готовим exporter

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Component
metadata:
  name: local-blackbox
spec:
  driver: blackbox
  mode: external
  agentSelector:
    matchLabels: {role: probe}
  endpoint: http://blackbox:9115
  moduleSet: standard-network
  enabled: true
```

| Поле `spec.*` | Тип | Обов. | Default | Призначення |
|---|---|---|---|---|
| `driver` | enum `blackbox` | Так | — | Зареєстрований adapter; інші drivers потребують нової реалізації/схеми. |
| `mode` | enum `external` | Ні | `external` | Exporter уже встановлено разом із Agent Stack або окремо. `managed` **не підтримується в v1alpha1**. |
| `agentSelector` | `LabelSelector` | Так | — | На яких агентах цей зовнішній компонент очікується. |
| `endpoint` | `http://` URL | Так | — | Локальний адресний endpoint в **Agent Docker network**. Публічні exporter URL за замовчуванням заборонені. |
| `moduleSet` | string | Ні | `standard-network` | Назва перевіреного набору Blackbox modules, який підготовлений у deployment templates. |
| `enabled` | boolean | Ні | `true` | Керує використанням binding; **не** запускає/встановлює контейнер. |
| `description` | string | Ні | `""` | Коментар. |

**Необхідне передумовлення:** `moduleSet` має існувати в компонентному каталозі закріпленого release. У v1alpha1 не можна записати довільний Docker image, команду інсталяції, shell script, URL плагіна чи необмежений вміст `blackbox.yml`. Якщо Component відсутній/недоступний на агенті, він не має відповідної capability: `runner_unavailable` — технічна помилка, не `Target DOWN`.

Для першого NetMonOrc release `Component` може автоматично створюватися **як зафіксований приклад конфігурації разом із Compose**, але його сутність залишається декларативною; `apply` змінює лише дозволений binding, а не життєвий цикл контейнера.

---
## 18. CEL — типізована умовна логіка, без довільного коду

CEL застосовується тільки там, де в таблиці поля явно вказано `CELExpression`: `AssessmentRule.when`, `DiagnosticBinding.when`, `FixedStep.when`, `AlertRule.when` / `resolveWhen`. **Немає** загального `eval:` у будь-якому місці YAML.

### 18.1. Дозволені контексти

| Контекст | Де доступний | Поля | Значення |
|---|---|---|---|
| `checks` | Zone assessment, Fixed DAG, Alert | `map<CheckID, CheckView>` | Лише актуальні придатні результати **конкретної зони**. Для відсутнього ID — відсутнє значення, не `healthy`. |
| `checks["id"].outcome` | Zone assessment, Fixed DAG | `success`,`failure`,`unknown` | Нормалізований результат перевірки; execution error не перетворюється на target failure. |
| `checks["id"].executionStatus` | Zone assessment, Fixed DAG | `completed`,`failed`,`not_executed` | Чи була виконана сама процедура. |
| `checks["id"].reasonCode` | Zone assessment, Fixed DAG | string | Наприклад `HTTP_UNEXPECTED_STATUS`, `TARGET_TIMEOUT`, `RUNNER_UNAVAILABLE`. |
| `checks["id"].values` | Zone assessment, Fixed DAG | `map<string, typed scalar>` | Результати `extract`, відсутні ключі не мають вигаданих значень. |
| `checks["id"].metrics` | Zone assessment, Fixed DAG | `map<string, number>` | Нормалізовані метрики (`latencyMs`, `httpStatus` тощо). |
| `checks["id"].ageSeconds` | Zone assessment, Fixed DAG | number | Вік Observation відносно часу оцінювання; для прострочених результати не вважаються придатними. |
| `zone` | Zone assessment, zone-scope Alert | `name`, `state`, `coverage`, `labels` | Мережева перспектива, для якої обчислюється правило. Під час первинної оцінки `state` ще не визначено; звернення до нього в `AssessmentRule.when` заборонене. |
| `monitor` | DiagnosticBinding, Alert, Fixed DAG | `name`, `state`, `coverageState`, `weightedScore?`, `labels` | `MonitorAssessment` після завершення зональної оцінки; у `AssessmentRule` `monitor.state` заборонений (усунення рекурсії). |
| `target` | Assessment, Diagnostics, Alert | `address?`, `type`, `networkZone`, `criticality`, `labels`, `attributes` | Похідний від inline target або `Target` ref, з розділеними labels. |
| `tasks` | Fixed DAG | `map<StepID, TaskView>` | Завершені попередні кроки: `outcome`, `executionStatus`, `values`, `reasonCode`. |
| `diagnostic` | Fixed DAG | `attempts`, `remainingChecks`, `elapsedSeconds` | Лічильники бюджету й поточного епізоду. |

`?` означає **можливу відсутність** поля. Умови перед компіляцією типізуються, а доступ до необов'язкових значень здійснюється через допустимі перевірки наявності, наприклад `has(checks["https"].values.queueDepth)`; для `unknown` підтримується окреме правилами оброблення, без примусового приведення до `false`. Приклад `httpStatus` — числова метрика, не рядок.

### 18.2. Оператори, типи, безпека

| Можливість | Рішення v1alpha1 |
|---|---|
| Boolean, number, string, list, map, `==`,`!=`,`>`,`>=`,`<`,`<=`,`&&`,`||`,`!`, `in` | Дозволено після type-check. |
| `has(...)` / перевірка наявності | Дозволено лише в підтримуваній CEL-семантиці optional/map attributes. |
| Арифметика | Дозволено над числами, без нескінченностей/NaN. |
| Вбудовані CEL macro | Лише whitelist без зовнішніх side effects. |
| Виклики HTTP, shell, файлової системи, мережі, SQL, `exec` | **Заборонено**. |
| Довільні user-defined functions із YAML | **Заборонено**; нові чисті функції додаються лише кодом, тестами й версією реєстру. |
| Максимальна довжина виразу | 4096 символів. |
| Ліміти виконання | Обмеження CEL cost, timeout і кількості операцій на сервері; під час перевищення — `evaluation_error`, не `false`. |
| Версіонування | Схема типів CEL-контексту є частиною API-контракту DSL. |

**Приклади:**

```yaml
# Негативний HTTP за доступного TCP:
when: 'checks["https"].outcome == "failure" && checks["tcp443"].outcome == "success"'

# Захоплене значення існує і перевищує поріг:
when: 'has(checks["https"].values.queueDepth) && checks["https"].values.queueDepth > 100'

# Alert про деградацію:
when: 'monitor.state == "degraded"'

# Діагностичний крок залежить від попереднього:
when: 'tasks["verify-tcp"].outcome == "failure"'
```

**Політика обробки помилок:** на рівні Assessment відсутнє значення, нестача актуальних перевірок або CEL error дають `unknown`/`coverage_degraded` (крім уже підтвердженої негативної умови). На рівні Diagnostic Step крок із невизначеною умовою не запускається й фіксується як `blocked_by_unknown`. На рівні Alert `unknown` **не** означає автоматично `resolve`.

---

## 19. Runtime-контракти для реалізації (не `kind`, не вхідний YAML)

### 19.1. `Observation`

| Поле | Тип | Обов. | Що означає |
|---|---|---|---|
| `observationID` | UUID | Так | Стабільний ID конкретного отриманого результату; незмінний при retry delivery. |
| `runID` | UUID | Так | Логічний ID запланованого запуску; fallback attempts мають окремі `observationID`, спільний `runID` або correlation parent. |
| `monitorID` | string/UUID | Так | Якого Monitor стосується. |
| `checkID` | CheckID | Так | Який скомпільований Check виконувався. |
| `agentID` | UUID | Так | Зіставляється із **автентифікованим mTLS** відправником. |
| `networkZone` | string | Так | Controller-managed зона виконавця; не довіряти самопроголошеному zone без підтвердження. |
| `failureDomain` | string | Ні | Група відмов для кореляції незалежності. |
| `appliedRevision` | integer/string | Так | Фактично застосована ревізія плану перевірок. |
| `startedAt`, `finishedAt`, `receivedAt` | RFC3339 UTC | Так | Початок/кінець вимірювання та приймання Controller. |
| `durationMs` | integer ≥0 | Так | Монотонно виміряна тривалість, а не різниця нескоригованих wall clocks. |
| `executionStatus` | enum `completed`,`failed`,`not_executed` | Так | Чи відпрацював runner. |
| `probeOutcome` | enum `success`,`failure`,`unknown` | Так | Факт щодо цілі; `failed` execution → `unknown`, якщо не отримано фактичного outcome. |
| `reasonCode` | string | Так | Типізована причина (`HTTP_UNEXPECTED_STATUS`, `RUNNER_UNAVAILABLE`, тощо). |
| `metrics` | map<string,number> | Ні | latency, responseCode та інші нормалізовані метрики. |
| `values` | map<string,typed scalar> | Ні | Захоплені `extract` значення. |
| `extractionStatus` | map<string,`success`,`missing`,`error`,`not_applicable`> | Ні | Стан кожного extractor. |
| `diagnosticSessionID`, `diagnosticTaskID` | UUID | Умов. | Для позапланової діагностики, не для звичайного регулярного check. |
| `isReplay` | bool | Так | Історичні дані після offline; не є новим live-triger за замовчуванням. |

**Транспорт:** at-least-once + ідемпотентний `INSERT` по `observationID`; ACK означає durable-запис. У `Agent` — обмежений SQLite spool, backpressure, політика втрати старих записів із лічильником. Replay не може перезаписувати новіші зональні стани.

### 19.2. `ZoneAssessment`, `MonitorAssessment`, `Coverage`

| Об'єкт | Поля | Призначення |
|---|---|---|
| `ZoneAssessment` | `monitorID`, `zone`, `state`, `scoreH?`, `coverageQ`, `requiredMissing[]`, `reasonCodes[]`, `evidenceIDs[]`, `evaluatedAt`, `revision` | Стан з однієї мережевої перспективи. |
| `MonitorAssessment` | `monitorID`, `state`, `coverageState`, `zoneStates[]`, `reasonCodes[]`, `evidenceIDs[]`, `evaluatedAt`, `revision` | Загальний результат за визначеною політикою `zoneAggregation`. |
| `Coverage` | `requiredZones[]`, `observedZones[]`, `staleChecks[]`, `missingChecks[]`, `status` | Показує, наскільки система реально спостерігає ресурс, окремо від стану сервісу. |

### 19.3. `DiagnosticSession`, `DiagnosticTask`, `Alert`

| Об'єкт | Ключові поля | Стан |
|---|---|---|
| `DiagnosticSession` | `id`, `monitorID`, `profileRef`, `mode`, `triggerAssessmentID`, `budget`, `remainingHypotheses[]`, `evidenceIDs[]`, `startedAt`, `finishedAt`, `stopReason` | `created`,`running`,`completed`,`inconclusive`,`cancelled` |
| `DiagnosticTask` | `id`, `sessionID`, `actionID`, `agentID`, `deadline`, `attemptNo`, `observationIDs[]` | `pending`,`assigned`,`running`,`succeeded`,`failed`,`expired`,`cancelled` |
| `Alert` | `id`, `ruleRef`, `monitorID`, `zone?`, `severity`, `reasonCodes`, `evidenceIDs`, `startedAt`, `firedAt`, `resolvedAt` | `normal`,`pending`,`firing`,`resolved` |

`completed` у DiagnosticSession означає **коректне завершення визначеного сценарію**, а не обов'язкове підтвердження конкретної фізичної причини. `inconclusive` є допустимим результатом за суперечливих/недостатніх даних або вичерпаного бюджету.

### 19.4. Read-only `Agent` API view і enrollment

| Поле | Тип | Походження / призначення |
|---|---|---|
| `agentID`, `name` | UUID, string | Реєстрація через Master UI/CLI. |
| `certificateIdentity`, `certExpiresAt` | string, time | mTLS identity; не задається YAML. |
| `expectedIP` | IP/null | Опційний enrollment restriction; не використовується як постійний identity. |
| `networkZone`, `failureDomain`, `labels` | string/map | Довірені метадані, встановлені адміністратором у реєстрі. |
| `capabilities`, `runnerHealth` | arrays/map | Звіти агента, перевірені Controller/локальним adapter. |
| `onlineState`, `lastHeartbeat`, `appliedRevision` | runtime | Зв'язок і applied plan, не тотожне фізичному стану VPS. |

**Enrollment-процедура:** Master генерує високoентропійний одноразовий токен із TTL **20 хвилин**, зберігає тільки його хеш, опційно прив'язує до очікуваного IP та адміністраторського імені. Agent установлюється окремим Docker Compose-профілем, перевіряє TLS ідентичність Master, передає токен та CSR. Master атомарно витрачає токен, реєструє Agent, видає сертифікат; далі робота через mTLS. Зміна IP не означає втрату постійної ідентичності.

---

## 20. Семантичні правила, які повинні перевірятися між ресурсами

| Код | Перевірка | У разі порушення |
|---|---|---|
| `V001` | Відомі `apiVersion`,`kind`; відсутні невідомі поля | Відхилити. |
| `V002` | Унікальні `metadata.name` у межах `kind`, ID усередині ресурсу | Відхилити. |
| `V003` | Рівно один із `Monitor.target` / `targetRef` | Відхилити. |
| `V004` | `...Ref` та inline-об'єкт не вказані одночасно | Відхилити. |
| `V005` | Усі посилання ведуть на наявні ресурси правильного `kind` | Відхилити. |
| `V006` | Профілі не рекурсивні, параметри типізовані, після expansion checks IDs унікальні | Відхилити. |
| `V007` | `type/runner/config` узгоджені, параметри підтримує закріплена версія driver | Відхилити. |
| `V008` | `extract` лише для HTTP native runner, regex компілюється, group існує | Відхилити. |
| `V009` | `Check.timeout < interval`, допустимі часові межі | Відхилити. |
| `V010` | `Placement.replicas` і `distinctBy` сумісні з доступною кількістю zones/failureDomains | Якщо немає **зараз** допустимих агентів — статус `pending`/`partial`, а не помилка синтаксису; якщо логічно неможлива політика — відхилити. |
| `V011` | Assessment містить кожний enabled check рівно раз (за явного списку), ваги не нульові | Відхилити. |
| `V012` | CEL вирази скомпільовані й типізовані під потрібний контекст | Відхилити. |
| `V013` | Немає циклів `requires` DAG, немає самозалежностей | Відхилити. |
| `V014` | Fixed DAG є ациклічним, всі `actionRef`/`dependsOn` валідні | Відхилити. |
| `V015` | Adaptive action має `expectedOutcomes` для кожної гіпотези | Відхилити. |
| `V016` | Один активний `WebhookEndpoint`, AlertRule посилається на нього | Відхилити конфлікт. |
| `V017` | Не допускаються секрети у відкритих YAML полях (HTTP headers, URL credentials) | Відхилити. |
| `V018` | Компонент `external`; `managed`, shell commands і довільні образи відсутні | Відхилити. |
| `V019` | `ResourceGroup.weighted` лише з фіксованими `members`, всі їхні ваги вказані | Відхилити. |
| `V020` | `spec.status`, `metadata.uid/generation/resourceVersion` не надходять із YAML | Відхилити. |
| `V021` | Лише підтримувані `monitorRef`/selector, `scope` для AlertRule | Відхилити. |
| `V022` | Поточні дані для `healthy` відповідають freshness/revision/required/coverage | Runtime не повинен повертати `healthy`; помилку конфігурації не маскувати. |

**Два рівні валідації:** `validate` перевіряє форму, типи, CEL, локальні зв'язки та довідкову інформацію; `plan/apply` додатково перевіряє існування зовнішніх refs, політики доступу, сумісність runners і фактичний стан. Недоступний Agent сам по собі не робить YAML недійсним — це `Pending/Partial/OutOfSync` status. Не потрібно вимагати в `validate`, щоб на всіх VPS було запущено Docker.

---
## 21. Зв'язок DSL із графово-матричною моделлю розділу 2.1 магістерської

Матричне подання є **математичною проєкцією** декларативних ресурсів і runtime-даних. Воно не вимагає зберігати всі матриці у вигляді щільних таблиць в PostgreSQL: у коді це можуть бути списки ребер, індекси й розріджені структури.

| Математична сутність | Розмір | Звідки формується | Для чого |
|---|---|---|---|
| `R={r₁…rₙ}` | `n` ресурсів | `Target`, inline `Monitor.target`, `ResourceGroup` як окремі логічні вузли | Каталог об'єктів спостереження й контексту. |
| `P={p₁…pₘ}` | `m` checks | `Monitor.checks` + розгорнуті `CheckProfile` | Каталог конкретних перевірок. |
| `A={a₁…aₖ}` | `k` агентів | Runtime `Agent Registry` після enrollment | Точки виконання. |
| `D=[dᵢⱼ]` | `n×n` | `Dependency` та `Monitor.dependencies` | Напрямлені залежності: `rᵢ` залежить від `rⱼ`. |
| `Λ=[λᵢg]` | `n×q` | `metadata.labels`, `ResourceGroup.selector` | Належність об'єктів до конфігураційних груп. |
| `B=[bᵢⱼ]` | `n×m` | `Monitor` → скомпільовані checks | Які перевірки характеризують кожен ресурс; нульові рядки для контекстних ресурсів допустимі. |
| `C=[cₐt]` | `k×t` | `Agent.capabilities`, `Component.driver` | Які типи `t` check підтримує агент `a`. |
| `X=[xⱼₐ]` | `m×k` | `PlacementPolicy`, assignments/reconciliation | Фактичний план призначення checks агентам. |
| `Z=[zₐq]` | `k×q` | Controller-managed `Agent.networkZone` | Матриця належності агентів до зон. |
| `W⁽ᵐ⁾` | `n×m` | `Target.criticality`, `CheckSpec.priority`, базове `B` | Планові ваги/пріоритети за дефіциту ресурсів. |
| `W⁽ᵉ⁾` | `n×m` | `AssessmentPolicy.checks[].weight` | Внесок різних перевірок у агреговані показники. |
| `Y(t)` | `m×k` для одного часового зрізу | Актуальні `Observation` | Успішність, негативний результат або відсутність придатного спостереження; повний event payload зберігається окремо. |
| `Φ` | множина функцій | `AssessmentRule.when`, `AlertRule.when`, `DiagnosticProfile` | Формалізовані логічні правила. |

Для конкретного Monitor з `m` checks та `k` агентів:

\[
B_{ij}=1 \Leftrightarrow \text{check }j\text{ відноситься до ресурсу }i,
\qquad
X_{ja}=1 \Leftrightarrow \text{check }j\text{ призначено агенту }a.
\]

**Планове число виконань:** `K = B X`; **планове зональне покриття:** `K_z = B X Z`. Це не матриці успішності: реальне покриття потребує актуальних `Observation` і перевірки часової/ревізійної придатності.

Оціночні ваги нормуються у межах кожного Monitor; для check `j`, зони `z`, часу `t`:

\[
Q_z(t)=\frac{\sum_j w_jv_{jz}(t)}{\sum_j w_j},\qquad
H_z(t)=\frac{\sum_j w_jv_{jz}(t)h_{jz}(t)}{\sum_j w_jv_{jz}(t)}.
\]

Тут `v=1` лише для придатного актуального result; `h=1` для успішного, `0` для негативного результату. За відсутності придатних observations знаменник `H` дорівнює нулю й `H` лишається **невизначеним**, а не `0`. `Q` — повнота, `H` — частка успішних **наявних** доказів. Остаточний `ZoneAssessment.state` — результат `Φ`, ваг, обов'язковості та coverage, а не один лише `H`.

**Дві незалежні ваги:**

- `W⁽ᵐ⁾`: конфігураційний пріоритет виконання, який **не впливає на значення** `H`; може формуватися як `Bᵢⱼ × criticalityᵢ × priorityⱼ` із подальшим нормуванням у планувальнику. Це пропонований принцип, точний scheduler-алгоритм описується окремо.
- `W⁽ᵉ⁾`: `AssessmentPolicy.checks[].weight`; визначає лише оціночний внесок. **Required check** не компенсується сумою інших ваг.

**Labels і залежності — різні типи відношень:** `Λ` описує множини/вибірки, `D` — функціональні/топологічні залежності; спільний label не є причинною залежністю. `ResourceGroup.mode: context` дає пасивний вузол; `mode: derived` дає похідний стан без власних probes.

---

## 22. Життєвий цикл YAML-конфігурації та межі відповідальності

| Крок | Модуль NetMonOrc | Вхід | Вихід / гарантія |
|---|---|---|---|
| 1. Decode | YAML Decoder | Один або кілька YAML-документів | AST/структури, координати помилок, відхилення дублів. |
| 2. Dispatch | Resource Registry | `apiVersion`, `kind` | Конкретна версія типу й її schema/validator/compiler. |
| 3. Schema Validate | Structural Validator | Сировинний `spec` | Типи, enum, обов'язковість, unknown-fields. |
| 4. Semantic Validate | Domain Validator | Ресурси і посилання | Ref resolution, типи, профілі, ациклічність, CEL, driver support. |
| 5. Normalize | Normalizer | Валідовані ресурси | Defaults, відсортовані правила, expansion профілів, стабільний hash. |
| 6. Compile | Monitoring Compiler | Нормалізований IR | Checks, placement constraints, assessment policies, diagnostic DAG. |
| 7. Plan | Placement + Reconciler | IR, Agent Registry | План змін `create/update/delete assignment`; без побічних дій. |
| 8. Apply | Resource Manager | План + затверджена версія | Desired revision у PostgreSQL; ідемпотентність. |
| 9. Assign | Controller | Desired revision, live agents | Assigned revision агентам; доставка по захищеному каналу. |
| 10. Acknowledge | Agent Config Manager | Скомпільований локальний план | Applied revision або помилка; попередній план зберігається при відмові. |
| 11. Execute | Local Scheduler + Probe Runner | Застосований план | Observations у локальний spool і Controller. |
| 12. Assess | Ingestion + Assessment Engine | Актуальні observations | Зональні стани, coverage, MonitorAssessment, evidence. |
| 13. Diagnose | Diagnostic Planner | Тригер та бюджет | Fixed/adaptive tasks або `inconclusive`. |
| 14. Alert | Alert Engine + Webhook Sender | Assessment + AlertRule | `pending/firing/resolved`, один стандартизований webhook. |

**Ревізії:** `desired` — прийнято Master; `assigned` — передано агенту; `applied` — агент підтвердив застосування. Це **не** одна атомарна операція для багатьох VPS. `Monitor` може перебувати у `PartiallyApplied` / `OutOfSync` до узгодження. Новий `apply` не повинен потайки перезапускати Docker Compose чи міняти системні credentials.

### 22.1. Як розширюється DSL без переписування ядра

| Новий елемент | Що додає розробник | Що **не** потрібно змінювати |
|---|---|---|
| Новий `kind` | `ResourceDefinition` у `ResourceRegistry`, схема, semantic validator, compiler, тести | YAML Decoder, API transport, базові metadata rules. |
| Новий тип check (`tls`, `snmp`) | `ProbeDefinition` / `ProbeDriver`, config-schema, capability та нормалізація результату | `Monitor` parser, загальне зберігання Observation. |
| Новий параметр існуючого check | Версіоноване розширення `ProbeConfig` + driver mapping + тести | Schema інших check types. |
| Новий extractor | Реєстрація `ExtractorDefinition`, schema та safe implementation | Інші `ExtractSpec` і CEL-компілятор. |
| Нова CEL-функція | Чиста типізована функція з whitelist, ліміти й tests | YAML Decoder, зовнішні exporters. |
| Новий backend зберігання метрик | Storage Adapter / Exporter | Схеми `Monitor`, діагностичний метод. |

У Go це може бути реалізовано як окремі registries для resource kinds, probe types, extractors та expressions. Неприпустимо проєктувати `ConfigEngine` як величезний `switch` із перевірками для всіх майбутніх параметрів. **Новий синтаксис вмикається лише після тестування й реєстрації**, а не через ігнорування невідомих полів.

---

## 23. Приклади маніфестів

### 23.1. Мінімальний Monitor: один HTTP check

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata:
  name: simple-http
  labels: {environment: test, service: web}
spec:
  target:
    type: endpoint
    address: example.com
  checks:
    - id: https
      type: http
      interval: 30s
      timeout: 5s
      config:
        url: https://example.com/
        expectedStatus: [200]
```

Діятиме стандартна placement policy (один допустимий agent), стандартні ваги й оцінювання, якщо немає іншого `AssessmentPolicy`.

### 23.2. Комплексний Monitor із профілем, вагами, двома зонами й adaptive diagnosis

**Це єдиний багатодокументний YAML-файл**. Приклад вимагає заздалегідь зареєстрованих агентів з `role: probe`, зонами `eu-west` і `us-east`, підготовленого Blackbox та Master-секрету `alert-webhook-token`. Адреси `example.com`/`192.0.2.0/24` — документаційні, не готовий production endpoint.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Target
metadata:
  name: corporate-api-target
  labels: {environment: production, service: web}
spec:
  type: service
  address: api.example.com
  networkZone: eu-west
  criticality: 5
---
apiVersion: netmonorc.io/v1alpha1
kind: Target
metadata:
  name: corporate-dns
  labels: {role: dns}
spec:
  type: service
  address: 192.0.2.53
---
apiVersion: netmonorc.io/v1alpha1
kind: Dependency
metadata:
  name: api-requires-dns
spec:
  from: {kind: Target, name: corporate-api-target}
  to: {kind: Target, name: corporate-dns}
  relation: resolvesVia
  strength: hard
---
apiVersion: netmonorc.io/v1alpha1
kind: CheckProfile
metadata:
  name: web-basic
spec:
  parameters:
    - {name: url, type: url, required: true}
    - {name: port, type: integer, default: 443}
  checks:
    - id: tcp
      type: tcp
      interval: 30s
      config: {port: '${params.port}'}
    - id: https
      type: http
      interval: 30s
      config: {url: '${params.url}', expectedStatus: [200]}
---
apiVersion: netmonorc.io/v1alpha1
kind: PlacementPolicy
metadata:
  name: two-regions
spec:
  selector:
    matchLabels: {role: probe}
  zones: [eu-west, us-east]
  replicas: 2
  distinctBy: zone
  strategy: leastLoaded
  onInsufficientAgents: partial
---
apiVersion: netmonorc.io/v1alpha1
kind: AssessmentPolicy
metadata:
  name: corporate-web-health
spec:
  requiredZones: [eu-west, us-east]
  zoneAggregation: all-required
  freshness: {maxAge: 90s, maxSkew: 25s}
  minCoverage: 0.80
  checks:
    - {id: reachability, weight: 0.10, required: false}
    - {id: web__tcp, weight: 0.30, required: false}
    - {id: web__https, weight: 0.60, required: true}
  rules:
    - id: http-fail
      priority: 100
      when: 'checks["web__https"].outcome == "failure"'
      then: {state: down, reason: HTTP_SERVICE_FAILED}
    - id: tcp-fail
      priority: 90
      when: 'checks["web__tcp"].outcome == "failure"'
      then: {state: degraded, reason: TRANSPORT_PROBLEM}
  weighted: {enabled: true, healthyMin: 0.90, degradedMin: 0.50}
  defaultState: unknown
---
apiVersion: netmonorc.io/v1alpha1
kind: DiagnosticProfile
metadata:
  name: web-adaptive
spec:
  mode: adaptive
  budget: {maxAdditionalChecks: 4, maxDuration: 60s, maxParallel: 2}
  hypotheses:
    - {id: dns-failure, description: DNS resolver issue}
    - {id: application-failure, description: HTTP application failure}
    - {id: zone-path-failure, description: Path depends on observation zone}
  actions:
    - id: dns-against-resolver
      check:
        type: dns
        timeout: 5s
        config:
          resolver: 192.0.2.53:53
          queryName: api.example.com
          recordType: A
      expectedOutcomes:
        dns-failure: failure
        application-failure: success
        zone-path-failure: success
      costUnits: 1
    - id: http-from-us
      check:
        type: http
        timeout: 5s
        config: {url: 'https://api.example.com/health', expectedStatus: [200]}
      placement:
        zones: [us-east]
        selector: {matchLabels: {role: probe}}
        replicas: 1
      expectedOutcomes:
        dns-failure: either
        application-failure: failure
        zone-path-failure: success
      costUnits: 1
  adaptive:
    algorithm: distinguish-pairs-per-cost
    stopWhenUnique: true
    maxNoGainSteps: 1
---
apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata:
  name: corporate-api
  labels: {environment: production, service: web}
spec:
  targetRef: corporate-api-target
  placementRef: two-regions
  useProfiles:
    - ref: web-basic
      as: web
      parameters: {url: 'https://api.example.com/health', port: 443}
  checks:
    - id: reachability
      type: icmp
      interval: 30s
      config: {preferredIPProtocol: ip4}
  assessmentRef: corporate-web-health
  diagnostics:
    enabled: true
    profileRef: web-adaptive
    onStates: [degraded, down]
    cooldown: 5m
---
apiVersion: netmonorc.io/v1alpha1
kind: ResourceGroup
metadata:
  name: production-web-services
spec:
  selector: {matchLabels: {environment: production, service: web}}
  mode: derived
  evaluation: {strategy: all-required, minCoverage: 1.0}
---
apiVersion: netmonorc.io/v1alpha1
kind: Component
metadata:
  name: blackbox-probes
spec:
  driver: blackbox
  mode: external
  agentSelector: {matchLabels: {role: probe}}
  endpoint: http://blackbox:9115
  moduleSet: standard-network
---
apiVersion: netmonorc.io/v1alpha1
kind: WebhookEndpoint
metadata:
  name: default-webhook
spec:
  url: https://alerts-gateway.example.net/netmonorc
  auth: {type: bearer, secretRef: alert-webhook-token}
  timeout: 5s
  retry: {maxAttempts: 5, initialBackoff: 2s, maxBackoff: 1m}
---
apiVersion: netmonorc.io/v1alpha1
kind: AlertRule
metadata:
  name: corporate-api-down
spec:
  monitorRef: corporate-api
  scope: monitor
  when: 'monitor.state == "down"'
  resolveWhen: 'monitor.state == "healthy"'
  severity: critical
  for: 60s
  channelRef: default-webhook
  notifyEvents: [firing, resolved]
```

**Результат:** конфігурація створює один Monitor із трьома базовими checks, дві потрібні зони, вагову оцінку, контекстну залежність, адаптивний каталог додаткових перевірок, логічну групу та єдиний webhook. При `apply` не створюються нові Docker-контейнери — Controller зіставляє бажані перевірки з уже встановленими/зареєстрованими Agents та Components.

### 23.3. HTTP extraction + CEL-умова, без прихованих подвійних запитів

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata:
  name: queue-depth
  labels: {environment: test, service: api}
spec:
  target: {type: service, address: queue.example.com}
  checks:
    - id: health
      type: http
      runner: native-http
      interval: 30s
      timeout: 5s
      config:
        url: https://queue.example.com/health
        method: GET
        expectedStatus: [200]
        verifyTLS: true
        maxBodyBytes: 8192
      extract:
        - name: queueDepth
          from: response.body
          using: regex
          pattern: 'queue_depth=(?P<value>[0-9]+)'
          group: value
          as: integer
          onMissing: unknown
  assessment:
    checks:
      - {id: health, weight: 1.0, required: true}
    rules:
      - id: queue-overloaded
        priority: 100
        when: 'has(checks["health"].values.queueDepth) && checks["health"].values.queueDepth > 100'
        then: {state: degraded, reason: QUEUE_OVERLOADED}
    weighted: {enabled: true, healthyMin: 0.9, degradedMin: 0.5}
    defaultState: unknown
```

**Семантика:** `native-http` виконує один GET, обмежує body, перевіряє HTTP status, витягує `queueDepth`, а далі CEL-правило може уточнити стан. За відсутнього `queueDepth` не підставляється `0` і не доводиться здоров'я черги. Пріоритети/інваріанти `required`, `freshness` і `coverage` застосовуються, як і для всіх Monitor.

### 23.4. Fixed diagnostic DAG — еталон для порівняння з adaptive

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: DiagnosticProfile
metadata:
  name: web-fixed-diagnostics
spec:
  mode: fixed
  budget: {maxAdditionalChecks: 4, maxDuration: 60s, maxParallel: 1}
  actions:
    - id: tcp-other-zone
      check:
        type: tcp
        timeout: 5s
        config: {port: 443}
      placement:
        zones: [us-east]
        selector: {matchLabels: {role: probe}}
        replicas: 1
    - id: dns-resolver
      check:
        type: dns
        timeout: 5s
        config:
          resolver: 192.0.2.53:53
          queryName: api.example.com
  fixed:
    steps:
      - id: verify-tcp
        actionRef: tcp-other-zone
      - id: verify-dns
        actionRef: dns-resolver
        dependsOn: [verify-tcp]
        when: 'tasks["verify-tcp"].outcome == "failure"'
```

Цей профіль можна підключати через `Monitor.spec.diagnostics.profileRef`, не змінюючи регулярні checks. Fixed і adaptive режими отримують той самий облік додаткових перевірок і результати одного формату.

---
## 24. Повний перелік default-поведінки і пріоритетів

| Ситуація | Дія компілятора / runtime |
|---|---|
| `Monitor.enabled` не задано | `true`. |
| Check `runner` не задано | `auto` → static runner selection (§5.5). |
| Check `interval`, `timeout` не задано | `30s`, `5s` (перевірка `timeout < interval`). |
| Check `jitter` не задано | `0s`; Scheduler може використовувати внутрішній не-конфігурований захист від синхронних стартів лише якщо це задокументовано як окремий runtime policy. |
| Check `priority` не задано | `50`. |
| `Monitor.placement` / `placementRef` не задані | Один online/допустимий least-loaded агент; за відсутності — `Pending`, не «успішний probe». |
| `AssessmentPolicy` не підключена | Рівні ваги enabled checks; `minCoverage=1`, `zoneAggregation=all-required`, `sameZoneAggregation=all`, required=false, зважені пороги `0.9/0.5`, `defaultState=unknown`. |
| `AssessmentPolicy.checks` задано | Усі enabled IDs мають бути оголошені рівно раз; автоматичне додавання відсутніх **заборонене**. |
| `requiredZones` не задано | Поточний набір призначених зон; його зміни видно через revision/coverage. Для стабільного SLA необхідно задавати явно. |
| `Monitor.diagnostics` не задано | Додаткова діагностика не запускається. |
| `ExecutionFallback` не задано | Технічні помилки фіксуються без автоматичного перепризначення; можливе окреме reconciliation за системними правилами, без приховування original. |
| `Extract.onMissing` не задано | `unknown`, без нульового значення. |
| CEL field відсутній або тип не збігається | Валідаційна помилка в `validate` для статично відомих полів; runtime `unknown/evaluation_error` для відсутніх data values. |
| `AlertRule.for` не задано | `0s`, firing одразу після підтвердженого condition (з урахуванням scheduler/latency). |
| `AlertRule.resolveWhen` не задано | Для `scope: monitor`: `monitor.state == "healthy"`; для `scope: zone`: `zone.state == "healthy"`. |
| Webhook не налаштований | Оцінювання/історія алертів можуть працювати, але активне `AlertRule` з відсутнім `channelRef` не проходить semantic validation; не відправляти повідомлення в довільну адресу. |
| Runtime Agent офлайн | Оновити `Agent` status, після TTL — coverage degraded; не робити `Target DOWN` автоматично. |
| Новий plan/revision | Попередні результати залишаються історичними; live Assessment користується даними актуальної застосованої ревізії. |
| За діагностики вичерпано бюджет | `DiagnosticSession.inconclusive` + `BUDGET_EXHAUSTED`; регулярний monitoring продовжується. |
| `ResourceGroup.mode: context` | Група не має власного health; лише membership та фільтрація. |
| Пасивний `Target` не має Monitor | `unknown` (або no assessment), не `healthy`. |

### 24.1. Явні конфлікти, які **не** можна «виправляти» автоматично

- Два profiles розгорнули той самий check ID; `AssessmentPolicy` посилається на неіснуючий ID.
- HTTP check містить `extract`, але вказано `runner: blackbox`.
- `requiredZones: [us-east]`, але адміністратор явно заборонив усі US-агенти в placement: конфігурація розглядається як **логічно нездійсненна** — помилка semantic validation, а не постійний `Pending`.
- У DiagnosticProfile `mode: adaptive` не задано signatures гіпотез.
- У декларативному YAML є `status: healthy`, пароль, enrollment-токен, приватний ключ або Docker installation command.
- `AssessmentRule` повертає `healthy`, хоча required check має відомий негативний результат: runtime invariant не дозволяє такий висновок, і в status додається `REQUIRED_CHECK_FAILED`.

---

## 25. Deployment, секрети, реєстрація та Web UI — зв'язок із DSL

Ці параметри **не є** `kind: Monitor` і не повинні змішуватися з декларативною моделлю.

| Механізм | Як настроюється | Чому не в YAML моніторингу |
|---|---|---|
| Встановлення Master/Agent | `netmonorc install --role master` / `--role agent` або Docker Compose profiles | Це lifecycle Docker-хоста, а не стан Monitor. |
| Перший username/password | Інтерактивний bootstrap Master; пароль хешується Argon2id в PostgreSQL | Не експортувати секрет у Git або `Monitor.spec`. |
| Enrollment Agent | Master UI/CLI створює одноразовий токен на **20 хвилин**, опційно bound IP; агент вводить адресу Master та токен | Enrollment живе окремо від бажаних перевірок, не зберігати token у конфігах. |
| Agent trust | Сертифікат, виданий після TLS-перевіреного enrollment; mTLS для довгого з'єднання | Перевірене identity не можна створити простим YAML. |
| Labels Agent | Адміністратор у Agent Registry через Web UI/API | Агент не може самостійно призначити собі довірені дозволи. |
| Локальні exporter-інструменти | Docker Compose Agent Stack, версіонований Component Catalog | `Component` у v1alpha1 лише прив'язує існуючий endpoint. |
| Webhook bearer token | Docker secret / захищений mounted secret, посилання за `secretRef` | Не записувати credentials у YAML/спостереження/аудит. |
| UI palette | Web UI design tokens | Візуальна тема не є частиною DSL-стану мережі. |

**Зафіксована палітра Web UI:** primary `#2E8B57`, background `#FDF5E6`. Для `healthy`, `degraded`, `down`, `unknown`, `coverage_degraded` використовувати не лише колір, а текст/іконки; контраст та доступність UI перевіряються окремо.

---

## 26. Перевірочні сценарії для нормативної схеми

| ID | Сценарій | Очікуваний результат |
|---|---|---|
| `SC-01` | Простий HTTP Monitor за `runner:auto` | Компілюється один blackbox check, очікування 200. |
| `SC-02` | Один Monitor має ICMP/TCP/HTTPS | Створено три окремі checks, один логічний Monitor. |
| `SC-03` | Target оголошено, але Monitor відсутній | Target є контекстним, probe не запускається. |
| `SC-04` | ResourceGroup context із labels | Membership формується, власний health не вигадується. |
| `SC-05` | ResourceGroup derived | Стан визначається лише з актуальних підлеглих Monitor assessments. |
| `SC-06` | CheckProfile використано двічі з різними `as` | ID не конфліктують, typed parameters підставлено. |
| `SC-07` | CheckProfile з параметром не того типу | `validate` повертає помилку з YAML path. |
| `SC-08` | Невідоме поле `intervall` | Відхилити, не ігнорувати. |
| `SC-09` | ICMP неуспішний, HTTPS успішний, ICMP не required | Стан сервісу може бути healthy за політикою, ICMP failure доступний як evidence. |
| `SC-10` | HTTPS 503, TCP успішний, HTTPS required | Не дозволено підтвердити healthy через ваги TCP/ICMP. |
| `SC-11` | EU failed, US success | Два zonal states збережено; глобальна політика `all-required` не приховує EU failure. |
| `SC-12` | EU offline, US success | `coverage_degraded` для EU, без автоматичного `Target DOWN`. |
| `SC-13` | Після outage приходить replay минулої ревізії | Дані в історії, але live Assessment не відкотиться. |
| `SC-14` | Exporter offline, target online | `executionStatus: failed`, `probeOutcome: unknown`; технічний fallback допускається. |
| `SC-15` | TCP target timeout | Негативний/неоднозначний target outcome за контрактом, не `runner_unavailable`. |
| `SC-16` | Regex не знайшов `queueDepth` | `extractionStatus: missing`; value відсутнє, не `0`. |
| `SC-17` | Regex invalid у YAML | `validate` відхиляє ще до планування. |
| `SC-18` | CEL посилається на неіснуючий check | Помилка compilation/semantic validation. |
| `SC-19` | Fixed DAG із циклом | Відхилити. |
| `SC-20` | Adaptive із двома нерозрізнюваними гіпотезами | Повернути сумісні гіпотези/`inconclusive`, не вигадувати причину. |
| `SC-21` | Diagnostic max checks вичерпано | Зупинка з `BUDGET_EXHAUSTED`, baseline probes не витісняються. |
| `SC-22` | AlertRule 60s, коротка помилка 10s | `pending` може з'явитися, `firing` не настане. |
| `SC-23` | Webhook тимчасово повертає HTTP 500 | Durable retry, стабільний `event_id`, один логічний alert event. |
| `SC-24` | Один enrollment token використано вдруге | Відхилити `TOKEN_ALREADY_USED`; API не створює другого Agent. |
| `SC-25` | Перевірка звертається до cloud metadata IP / redirect | Відхилити локальною target policy. |
| `SC-26` | `apply` без зміни нормалізованого spec | Не створювати нову semantic revision. |
| `SC-27` | Два агенти в одній зоні дають різні results | Застосувати `sameZoneAggregation`, зберегти divergence/evidence. |
| `SC-28` | Зміна Agent IP після enrollment | mTLS identity продовжує діяти за чинної політики; IP не використовується як єдина довіра. |

**Для наукового експерименту:** окремо порівнювати static, fixed DAG та adaptive за однакових baseline checks, доступних agents/actions і бюджетів. Вимірювати точність/частку `inconclusive`, latency та кількість додаткових probes. Ці сценарії не доводять новизну автоматично, але забезпечують відтворюваність реалізації.

---

## 27. Що реалізувати програмно з цієї специфікації

### 27.1. Рекомендовані Go-підсистеми

| Підсистема | Публічний внутрішній контракт | Артефакт |
|---|---|---|
| `yaml/decoder` | `DecodeDocuments(io.Reader) ([]RawResource,error)` | Розбір YAML, збереження шляху до помилки, duplicate-key detection. |
| `dsl/registry` | `Register(ResourceDefinition)` / `Resolve(apiVersion,kind)` | Реєстр підтримуваних `kind`. |
| `dsl/schema` | `ValidateStructure(RawResource)` | JSON Schema Draft 2020-12-подібні описи для кожного `kind`, закриті поля. |
| `dsl/semantic` | `ValidateReferences/ValidateCEL/ValidateGraph` | Складні інваріанти `V001…V022`. |
| `dsl/normalizer` | `Normalize(Resource) (CanonicalResource,error)` | Defaults, typed profiles, order-insensitive canonical hash. |
| `dsl/compiler` | `Compile(CanonicalResourceSet) (MonitoringIR,error)` | Виконуваний граф і політики, незалежні від формату YAML. |
| `drivers/registry` | `Register(ProbeDefinition)` | Схеми `ProbeConfig` і перевірки capabilities. |
| `drivers/blackbox` | `Execute(ctx,ProbeRequest)` | Локальні експортерні probes. |
| `drivers/nativehttp` | `Execute(ctx,ProbeRequest)` | Bounded HTTP + extraction. |
| `rules/cel` | `CompileTyped` / `EvaluateBounded` | Безпечна CEL-логіка. |
| `resources/repository` | CRUD + optimistic concurrency | Декларативні ревізії у PostgreSQL. |
| `controller/reconcile` | `Diff(desired,assigned,applied)` | Передавання планів агентам. |
| `assessment` | `Assess(ObservationSet,AssessmentSpec)` | Зональний і глобальний стан. |
| `diagnostics` | `NextAction(Evidence,Hypotheses,Budget)` | Fixed/adaptive планування. |
| `alerting` | `Transition(AlertRule,Assessment)` | Pending/firing/resolved та outbox. |

**Важливий принцип масштабування:** `YAML Decoder` не містить knowledge про ICMP, TCP, regex чи CEL. Нові поля додаються до відповідного `ResourceDefinition` або `ProbeDefinition`, а нові типи результатів нормалізуються в стабільний `Observation` контракт.

### 27.2. Рекомендовані файли схем

```text
schemas/
  v1alpha1/
    envelope.schema.json
    common/
      metadata.schema.json
      selector.schema.json
      placement.schema.json
      check.schema.json
      extraction.schema.json
      fallback.schema.json
      assessment.schema.json
    resources/
      target.schema.json
      monitor.schema.json
      check-profile.schema.json
      placement-policy.schema.json
      assessment-policy.schema.json
      diagnostic-profile.schema.json
      alert-rule.schema.json
      resource-group.schema.json
      dependency.schema.json
      component.schema.json
      webhook-endpoint.schema.json
```

**Це структура для реалізації, а не вже згенеровані JSON Schema-файли.** Структурні схеми перевіряють типи та форму, semantic validators — посилання, consistency, CEL і можливості зовнішніх drivers.

---

## 28. Зовнішні технічні орієнтири

Цей YAML DSL є **власним проєктом NetMonOrc**, а не синтаксисом наведених інструментів. Для реалізації конкретних adapters/validators треба звіряти документацію **за версіями закріплених залежностей**.

| Технологія | Що повторно використовуємо | Документація |
|---|---|---|
| Prometheus Blackbox Exporter | HTTP, DNS, TCP, ICMP та module configuration | [README](https://github.com/prometheus/blackbox_exporter/blob/master/README.md), [CONFIGURATION.md](https://github.com/prometheus/blackbox_exporter/blob/master/CONFIGURATION.md) |
| CEL (`cel-go`) | Обмежені типізовані вирази для оцінювання/умов | [CEL](https://cel.dev/), [cel-go](https://github.com/google/cel-go) |
| JSON Schema | Структурна перевірка ресурсів, definitions/refs, заборона зайвих полів | [JSON Schema](https://json-schema.org/specification) |
| Docker Compose | Різні Master/Agent deployment profiles | [Compose profiles](https://docs.docker.com/compose/how-tos/profiles/) |
| Go regexp | Безпечне лінійне зіставлення regex для обмежених body | [regexp](https://pkg.go.dev/regexp) |

---

## 29. Підсумковий контракт

**Завершена схема v1alpha1** охоплює:

- **11 декларативних типів**: `Target`, `Monitor`, `CheckProfile`, `PlacementPolicy`, `AssessmentPolicy`, `DiagnosticProfile`, `AlertRule`, `ResourceGroup`, `Dependency`, `Component`, `WebhookEndpoint`;
- вкладені блоки `CheckSpec`, `ProbeConfig`, `PlacementSpec`, `ExtractSpec`, `ExecutionFallback`, `AssessmentSpec`, `DiagnosticAction`, `DiagnosticBudget`, CEL-правила;
- read-only runtime-об'єкти `Agent`, `Observation`, `Assessment`, `DiagnosticSession`, `DiagnosticTask`, `Alert`, `ConfigRevision`;
- принципи активних, контекстних і похідних ресурсів;
- окремі ваги **планування** й **оцінювання**, зональну модель і coverage;
- формалізовану семантику `unknown`, `failed execution`, `failed probe`, `fallback`, `replay` та конфігураційних ревізій;
- один зовнішній webhook і інтеграцію зі стандартними exporters;
- стандартизоване розширення DSL шляхом версіонованого реєстру ресурсів, драйверів і extractor-ів.

**Необхідний наступний інженерний артефакт:** на основі цього документа створити машиночитані JSON Schemas та Go-моделі, після чого перевірити всі наведені маніфести інтеграційними тестами і зафіксувати перший релізний контракт `netmonorc.io/v1alpha1`.
