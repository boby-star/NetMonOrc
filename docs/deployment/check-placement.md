# Placement за labels/domains та окремими checks

Controller приймає `-agent-labels-file=/private/agent-labels.json`:

```json
{"branch":{"role":"probe","site":"office"},"dc":{"role":"probe","site":"server"}}
```

Відомі домени задаються наявним `-failure-domains=branch=uplink-a,dc=uplink-b`.
Обидва інвентарі належать Controller. Hello/heartbeat Agent не можуть їх змінити.
Файл містить лише labels відомих roster IDs; JSON duplicate keys, невідомі Agent,
null, перевищення меж або некоректні labels відхиляються. Labels є також у read
API Agent/React. Зміни startup inventory застосовуються під час перезапуску
Controller; автоматичного file watcher немає. Offline validate може отримати
цей файл через `--agent-labels-file` разом зі статичним `--slots`.

## Селектори та домени

```yaml
placement:
  selector:
    matchLabels: {role: probe}
    matchExpressions:
      - {key: site, operator: In, values: [office, server]}
  replicas: 2
  strategy: leastLoaded
  distinctBy: failureDomain
```

selector, agentRefs та zones поєднуються через AND. Підтримано In/NotIn/Exists/
DoesNotExist з перевіркою key/operator/values. Порядок expressions і множин values
канонізується; він не змінює policy hash. Default strategy лишається leastLoaded;
fixed/roundRobin також підтримують нові обмеження. AgentRefs/zone/capabilities,
онлайн-стан, місткість та CIDR policy не скасовуються селектором.

Failure domain береться з довіреного mapping, не із zone/Agent ID або самозвіту.
За спільного чи невідомого домену потрібний незалежний набір не утворюється.
Зберігається pending без часткових probes. Історичний план містить використані
домени; Agent перевіряє distinctness і цілісність плану. Потрібна capability
placement:domains. Зміна допустимих кандидатів/domains входить до effective
binding hash і не дозволяє застосувати старий кеш від іншого inventory.

## Окреме розміщення кожної перевірки

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata: {name: split-checks}
spec:
  target: {address: 198.18.0.80}
  checks:
    - id: http-office
      type: http
      interval: 10s
      timeout: 2s
      config: {url: 'http://198.18.0.80:8080/', expectedStatus: [200]}
      placement:
        selector: {matchLabels: {site: office}}
        replicas: 1
        strategy: leastLoaded
    - id: tcp-server
      type: tcp
      interval: 10s
      timeout: 2s
      config: {port: 9090}
      placementRef: server-policy
  assessment:
    checks:
      - {id: http-office, weight: 1, required: true}
      - {id: tcp-server, weight: 1, required: true}
---
apiVersion: netmonorc.io/v1alpha1
kind: PlacementPolicy
metadata: {name: server-policy}
spec:
  selector: {matchLabels: {site: server}}
  replicas: 1
  strategy: roundRobin
```

Check.placement/ref перевизначає Monitor placement лише для цього check.
Обидва одночасно заборонені. Це працює і після expansion CheckProfile; шаблон
перевіряється при створенні, а refs остаточно зв’язуються в каталозі Monitor.
Вимкнені checks не створюють призначень. Видалення PlacementPolicy, на яку
посилаються checks Monitor/CheckProfile, блокується.

Кожен check має власні кандидати/вибір/домен/зональний scope. Agent отримує
загальний контрольний план, але виконує тільки дозволені йому check IDs.
Відсутність призначення одного check не забороняє іншим виконуватися.
Capability placement:per-check запобігає мовчазному виконанню старим Agent.
Config plan/CLI/React preview містить окремі рядки з checkId/strategy/distinctBy.

Assessment рахує planned N, all/any/quorum, required та physical coverage за
джерелами конкретної перевірки в конкретній зоні. Check, запланований в іншій
зоні, не є пропущеною replica. Required check поза всіма явно оцінюваними зонами
не дозволяє підтвердити healthy; показується required_check_outside_assessment.
Різні checks на різних Agent однієї зони не стають кількома replicas кожного.
Значення CEL не усереднюються; справжні same-zone multi-replica values/metrics
і далі обмежені раніше погодженим контрактом.

LeastLoaded/roundRobin per-check вибір зберігається в nmo_check_placements;
shared PlacementPolicy використовує свій durable cursor, inline — Monitor UID
і check ID. Зміна графа/target/runner спершу вилучає застарілий admission.
Template hash розраховується без cached selections, щоб залежності між
кількома checks не утворювали циклу ревізій. Повторний heartbeat і restart
із тим самим допустимим вибором не створюють нову ревізію.

## Доставка та міграція

AssignPlan зберігає дозволені check IDs у nmo_assignments.allowed_checks.
CommitBatch перевіряє їх навіть для replay старої ревізії, коли поточний runtime
вже не має старого Plan. Діагностичні tasks мають свою окрему перевірку походження.
Старі записи з NULL allowed_checks залишаються legacy без вигаданого списку;
нові per-check плани створюють точний список, зокрема порожній для idle Agent.

Міграції додають таблицю та nullable колонку; існуючі дані/volumes не видаляються.
Це не додає enrollment/HA або автоматичного визначення labels/topology.
[Натурні докази](../experiments/check-placement-2026-11-19/README.md).
