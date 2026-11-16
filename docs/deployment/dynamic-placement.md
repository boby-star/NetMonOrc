# Динамічне призначення регулярних перевірок

На довіреному fixed roster доступні `leastLoaded` та `roundRobin` поряд із
попереднім `fixed`. Це Monitor-level placement: один обраний набір виконує
регулярні checks цього Monitor. Partial placement залишається вимкненим.
Нових DSL fields або HTTP routes немає.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: PlacementPolicy
metadata: {name: regular-pool}
spec:
  strategy: roundRobin
  agentRefs: [branch, dc]
  replicas: 1
  distinctBy: zone
  onInsufficientAgents: pending
```

У Monitor задайте `placementRef: regular-pool` або чинне inline `placement`.
Якщо strategy у заданому PlacementSpec пропущено, використовується нормативний
`leastLoaded`. AgentRefs можна не задавати: кандидатами буде довірений roster,
обмежений policy.zones. Відсутність самого placement/ref зберігає попередній
статичний режим стенда; повний нормативний compiler-default ще не підключений.

## Відбір і стійкість

Controller перевіряє identity/zone зі свого roster, свіжий heartbeat, строк
сертифіката, фактичні capacity/active gauge, runner/type/extraction capabilities
та локальний дозвіл на всі цілі Monitor. Agent рекламує свій незмінний на час
процесу allowlist як capabilities `target-cidr:<CIDR>` через захищений Hello.
Це не довільний label selector і не право YAML розширювати allowlist. Controller
використовує ту саму перевірку IP-literal target policy, що й probe runners;
Agent повторно перевіряє її перед запуском. Reachability не вгадується з allowlist.
Невідома місткість, відсутня реклама policy або заборонена ціль виключають Agent.

При новому призначенні `leastLoaded` порівнює active/capacity; рівність вирішує
agentID ASC. `roundRobin` обходить відсортовану feasible множину зі збереженого
покажчика конкретної policy. Для placementRef покажчик спільний між Monitor,
для inline policy — належить Monitor. Покажчик змінюється лише за повного нового
призначення, а не на кожному heartbeat. PostgreSQL зберігає його через restart.

Чинний повний feasible набір зберігається, навіть якщо короткий gauge іншого
Agent став нижчим. Втрата придатності джерела запускає новий відбір; повернення
Agent не переміщує робочі призначення назад. Якщо кандидатів недостатньо,
selection порожній: не виконуються навіть доступні часткові replicas.

Після запуску runtime Controller чекає 15 секунд перед новим відбором, щоб
отримати ACK/heartbeat після загального reconnect. Уже збережені selections
передаються одразу; ця пауза не продовжує їхні lease. Втрата Agent визначається
чинним heartbeat timeout; безперервні зміни catalog можуть відкладати відбір,
оскільки незалежний hot reload ще не реалізований.

## Ревізії, coverage та apply

Повний resolved plan включає policy hash, дозволених кандидатів і selection.
Зміна selection створює нову effective revision відповідного Monitor та залежних
ResourceGroup, не змінюючи source manifest/resourceVersion. Нові дані потрібні
саме з цієї ревізії. Історичний replay не відновлює старий live стан.

`apply --wait` відстежує causally recorded placement-переходи тієї самої resource
identity та source version. Тільки після ACK потрібних Agent operation може
стати completed. Сторонні зміни manifest/dependencies не вважаються placement
переходами й надалі дають CONFIG_SUPERSEDED. Pending набір не дає completed.
Покажчик, selection, залежні hashes і зв'язки переходів оновлюються атомарно
під тим самим catalog lock, що й apply. Видалення Monitor прибирає його selection.

Явні assessment.requiredZones зберігають пріоритет. Перепризначення у server
не замінює доказ з office, якщо policy оцінювання явно вимагає office. За
відсутності requiredZones defaults виводяться з поточного повного selection;
поки placement pending — з дозволених candidate zones. Втрата monitoring
джерела означає unknown/coverage degradation, а не підтверджену відмову сервісу.

Збережений selection додатково прив'язаний до hash повного розгорнутого
Monitor без selection. Зміна target, runner, checks чи залежних profiles
спочатку переводить його в pending і вимагає нового перевіреного відбору;
самої рівності policy hash недостатньо для повторного дозволу запусків.

## Межі

R32 додає [кілька Agent на зону та all/any/quorum](replica-assessment.md).
Replicas 1–10, явний distinctBy=zone зберігає різні зони.
trusted label selectors, distinctBy=failureDomain, per-check overrides,
Agent enrollment і автоматичне балансування вже feasible наборів ще відкриті.
Локальні runners підтримують лише IP-literal цілі; hostname-based target
policy не додається цим етапом. Без placement/ref лишається legacy режим.
Старі Agent без placement:dynamic/target-cidr advertisement не отримують
динамічне призначення; оновлюйте Controller та Agent разом.

Критичний сценарій: `scripts/lab_sessions.py --dynamic-only` на свіжому owned
стенді. Він перевіряє два roundRobin Monitor, restart, втрату Agent, збереження
чужої ревізії, pending без фізичних probes, default leastLoaded та cleanup.
Це HTTPS/CLI integration, а не браузерна взаємодія чи performance benchmark.
