# Повторно використовувані CheckProfile (R22)

CheckProfile підключено до чинного каталогу ресурсів REST, CLI та React.
Використовується DSL §4.3/§12 без нових полів: parameters, checks, description;
Monitor підключає профіль через useProfiles із ref, as та parameters.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: CheckProfile
metadata: {name: standard-web}
spec:
  parameters:
    - {name: url, type: url, required: true}
    - {name: expected, type: integer, default: 200}
  checks:
    - id: http
      type: http
      runner: blackbox
      interval: 10s
      timeout: 2s
      config:
        url: '${params.url}'
        expectedStatus: ['${params.expected}']
```

В існуючому Monitor перевірку можна замінити на:

```yaml
useProfiles:
  - ref: standard-web
    as: web
    parameters: {url: 'http://198.18.0.80:8080/'}
```

Підсумковий CheckID — web__http. Assessment і CEL повинні посилатися вже на
розгорнутий ID. Inline checks додаються до результату; колізії ID відхиляються,
overwrite немає. Підсумковий ID має максимум 64 символи; обрізання чи hashing ID
не застосовуються. Можна використовувати один профіль кілька разів із різними
as і параметрами, зокрема разом із Target/targetRef.

## Параметри й валідація

Підтримано string, integer, float, boolean, url, ip, hostname. Required parameter
потребує явного значення та не може мати default. Необов'язковий параметр бере
оголошений default; якщо його немає, значення не вигадується. Використання
такого параметра без binding/default дає помилку. Невідомі параметри й поля,
повторні імена, неправильні типи, null та непідтримані checks відхиляються.

Integer зберігається як int64 без перетворення на float. Float — скінченне
IEEE 754 значення з числового scalar, включно з інтегральним числовим записом;
рядки не перетворюються на числа чи bool. Оголошений тип float відновлюється
після нормалізації чисел у PostgreSQL JSONB. YAML/CLI зберігають відмінність
між integer і float; duplicate keys, NaN/Inf, YAML merge і alias cycles відхиляються.
Розгортання YAML обмежене 64 рівнями й 65536 nodes, числовий scalar — 4096 символами.

Лише ціла строкова форма ${params.name} всередині checks[].config може бути
підставлена. Вставлення частини URL/regex, зміна mapping keys, ID, runner,
interval, extraction або CEL через шаблон заборонені. Немає env/shell/Jinja
substitution, вкладених useProfiles чи рекурсивного виконання параметрів.

Профіль перевіряється до commit: оголошення, defaults, закрита схема checks
і сталі обмеження проходять чинний compiler. Для ще не заданих required значень
використовуються лише внутрішні typed validation witnesses. Вони не є defaults
і ніколи не потрапляють у план виконання. Кожен конкретний Monitor повністю
компілюється повторно після підстановки його фактичних значень. Саме ця перевірка
визначає допустимість конкретного URL/port/enum; зберегти invalid bound Monitor
не можна. Профіль використовує підтриманий поточним prototype піднабір CheckSpec;
непідключені placement/fallback можливості не оголошуються реалізованими.

## Desired graph та ревізії

CheckProfile і Monitor можна подавати одним mixed batch у будь-якому порядку.
Нормалізовані hash профілю та bound parameters входять до Monitoring IR.
CheckProfile-only update перераховує effective revision залежних Monitor,
не змінюючи їхній source manifest/resourceVersion. Plan показує reconcile,
apply додає implicit targets і чекає фактичних Agent ACK. Старі observations
залишаються історією, не відновлюють старий live assessment.

Профіль із посиланнями захищений від видалення (409). CRUD/history/status,
ETag/If-Match, scopes, cookie CSRF, audit та idempotency використовують спільний
механізм ресурсу. Сам CheckProfile не запускає probes. Як і Target IR bindings,
доповнення потребує узгоджених Controller/Agent одного випуску.

```sh
netmonorc get --context-file /private/context.json CheckProfile
netmonorc get --context-file /private/context.json CheckProfile standard-web
netmonorc plan --context-file /private/context.json -f resources.yaml
netmonorc apply --context-file /private/context.json -f resources.yaml --wait
```

У React виберіть CheckProfile у каталозі конфігурації. Валідація/компіляція
виконується Controller. UI не дублює правила параметрів чи оцінювання.
Початковий startup Monitor залишається inline bootstrap; persisted useProfiles
завантажується з PostgreSQL після restart і не перезаписується startup файлом.

## Критичні перевірки

Unit tests: typed defaults/bindings, whole-scalar substitution, ID collisions/length,
missing/unknown refs, canonical hash, YAML ambiguity/alias bounds та Agent VerifyPlan.
Real PG/HTTPS: mixed apply, JSONB numeric round trip, reference delete guard,
CheckProfile-only revision change та незмінна source version Monitor.
Compose: три generated checks виконуються двома Agent; зміна default
expectedStatus змінює фактичний assessment healthy→down, переживає restart,
відновлення профілю повертає healthy. DOM clicks не автоматизовані; React
перевіряється production build.
