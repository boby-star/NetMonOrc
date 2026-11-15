# Спільні Target і Monitor.targetRef (R21)

Target доступний у чинному REST каталозі, React-редакторі й CLI. Підтримані
поля DSL §3: type, address, networkZone, criticality, description, attributes.
Тип обов'язковий; address обов'язковий крім logical. Значення за замовчуванням:
networkZone=unspecified, criticality=3. Адреса — IP або DNS hostname, без URL;
IPv4-mapped адреси нормалізуються, DNS імена приводяться до нижнього регістру.
Description обмежений 1024 символами, attributes — 32 scalar-полями. Невідомі
поля, null, вкладені attributes, неправильні типи й діапазони відхиляються.

Створення Target не створює Monitor, задач чи спостережень. Target без Monitor
не отримує healthy. Logical Target без адреси дозволений як пасивний контекст,
але його не можна призначити поточному активному probe Monitor.

## Використання

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Target
metadata:
  name: corporate-service
spec:
  type: service
  address: 198.18.0.80
  networkZone: server-room
  criticality: 3
```

У Monitor замініть `spec.target: {address: ...}` на
`spec.targetRef: corporate-service`. Рівно один із цих двох полів обов'язковий.
Решта checks/assessment/diagnostics залишається у чинному підтриманому форматі.
Target і Monitor можна подати в одному mixed apply в будь-якому порядку.

```sh
netmonorc get --context-file /private/context.json Target
netmonorc get --context-file /private/context.json Target corporate-service
netmonorc plan --context-file /private/context.json -f resources.json
netmonorc apply --context-file /private/context.json -f resources.json --wait
```

У React: «Конфігурація моніторингу» → Target. CRUD/history/status,
ETag/If-Match, scopes, CSRF, audit та idempotency використовують спільну
реалізацію ресурсів. Target із наявними Monitor.targetRef захищено від
видалення (409); спочатку змініть або видаліть посилання.

## Ревізії й виконання

Compiler розгортає targetRef в адресу Monitoring IR до мережевого виконання.
У плані є внутрішні TargetRef/TargetHash, де hash розрахований із нормалізованого
Target.spec. Він включений у canonical effective revision. Labels цілі не
зливаються з labels Monitor; networkZone цілі не змінює зони перевірок;
criticality не перетворюється на вагу оцінювання чи severity.

Зміна Target.spec повторно компілює всі залежні Monitor. Власний manifest,
resourceVersion і history Monitor не змінюються через зміну залежності;
змінюється лише effective revision. Plan показує implicit reconcile targets,
а apply додає залежні Monitor до operation. Завершення чекає чинної ревізії
Controller і heartbeat-backed ACK від потрібних Agents.

До появи нових доказів стара оцінка не використовується як live assessment.
Agent перевіряє checksum розгорнутого плану та формальну коректність Target
binding. Його мережева CIDR/DNS policy залишається обов'язковою. HTTP URL
береться з check.config.url; targetRef не переписує явну URL-адресу HTTP check.
Для TCP/ICMP використовується розгорнута Target.address.

Startup --manifest у цьому прототипі залишається одиночним inline Monitor,
з якого визначається початковий fixed roster. Після першого імпорту desired
Target/Monitor керуються через apply й завантажуються з PostgreSQL після
restart; початковий inline файл їх не перезаписує. Внутрішнє доповнення IR
потребує відповідних Controller і Agent одного випуску. Multi-Monitor execution,
CheckProfile expansion та Dependency/ResourceGroup залишаються окремими роботами.

## Критичні перевірки

Unit tests перевіряють закриту схему, типи/defaults, passive logical Target,
oneOf, відсутній ref, зміни hash і приймання плану Agent-ом. PostgreSQL/HTTPS
use case перевіряє unordered mixed apply, 409 на видалення з посиланням,
Target-only dependency revision, незмінну версію manifest Monitor та fencing
старого healthy assessment. Live Compose сценарій підтверджує доставку
Target-bound плану двом реальним Agent, залежний ACK, нові спостереження,
persistence після restart, unlink та delete. React перевірено production build;
DOM clicks не автоматизовані.
