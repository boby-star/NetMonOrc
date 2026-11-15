# Фіксоване розміщення регулярних перевірок (R24)

PlacementPolicy доступний у REST-каталозі, CLI, React-редакторі та mixed apply.
Monitor приймає placementRef або inline placement, але не обидва одночасно.
Цей етап реалізує тільки явний strategy=fixed для одного активного Monitor.
Контракт DSL не змінено: непідтримані режими відхиляються при валідації.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: PlacementPolicy
metadata: {name: office-only}
spec:
  strategy: fixed
  agentRefs: [branch, dc]
  zones: [office]
  replicas: 1
  distinctBy: zone
  onInsufficientAgents: pending
```

У Monitor задайте `placementRef: office-only` і потрібні assessment.requiredZones,
наприклад `[office]`. Політику та Monitor можна створити одним mixed apply.
Наявні agentRefs повинні належати довіреному roster Controller, заданому через
`-slots branch=office,dc=server`; це статично дозволені mTLS identities, а не
API реєстрації нових Agent. Startup bootstrap зберігає явні початкові зони;
placement/ref застосовується через desired catalog.

Відбір: дозволені agentRefs → фільтр zones → Agent ID ASC → перші replicas.
Це детермінований fixed-вибір; online-state не змінює його автоматично.
Підтримуються replicas 1–10 у різних зонах, distinctBy=none/zone та pending.
Roster обмежено 100 identities, по одному Agent на зону. Повторні replicas в
одній зоні, failureDomain, непорожні selectors, per-check overrides,
leastLoaded та roundRobin ще не реалізовані. Якщо strategy пропущено,
нормативний default leastLoaded не підмінюється fixed: конфігурація відхиляється.
Partial вимкнений відповідно до погодженого рішення.

Якщо після фільтрів бракує Agent, Selected порожній: жоден regular probe не
стартує. Apply operation залишається running із причиною
insufficient_placement_agents, assessment має unknown/coverage_degraded.
Необхідні зони не вигадуються: для pending без явних requiredZones немає
підстав вивести defaults. За повного selection defaults беруть його зони.
Явні requiredZones можуть бути підмножиною bootstrap-зон; відсутній доказ
для обов'язкової зони зменшує coverage. Без placement/ref лишається попередній
prototype-режим: один regular Agent у кожній bootstrap-зоні. Це compatibility
режим прототипу, а не завершений нормативний default leastLoaded.

## Розмежування повноважень

Controller розрізняє всі дозволені identities, обрані regular sources і sources
для assessment.requiredZones. Усі довірені Agent отримують revisioned IR та
lease, підтверджують план і надсилають heartbeat. Необраний Agent залишається
online, але не запускає regular checks. Його separately authorized diagnostic
TaskAssignment може виконуватись за чинними бюджетами, deadline і target policy.

Новий optional Agent DTO field regularMonitoringAssigned показує бажане
призначення, а не online-state чи здоров'я сервісу. Поле відображається у React
та CLI. У React пакетне застосування має кнопку «Показати план пакета»: Monitor
і нову policy можна переглянути разом до запису desired. Зміна тексту скидає
старий preview. Plan.placements має типізований preview обраних identities/зон або
pending; preview не доводить online-state, target reachability чи capacity.
Operation completed означає ACK потрібних обраних Agent, не healthy сервісу.
Generic resource status active означає controller_loaded; для недостатнього
розміщення орієнтуйтесь на operation reason, preview та assessment coverage.

Plan hash включає policy hash, roster та selection. Policy-only change оновлює
залежний Monitor effective revision без зміни source manifest/resourceVersion.
Після restart desired policy/ref завантажуються з PostgreSQL. Залежну policy
не можна видалити (409); optimistic locking, CSRF, RBAC і idempotency спільні
з іншими ресурсами. Старі Agent без capability placement:fixed не допускаються.
Controller і Agent необхідно оновлювати разом.

Agent перевіряє власну identity/zone у compiled roster. Scheduler запускає
regular checks тільки за дозволеного selection та дійсного lease. Transport
відхиляє поточні regular observations від idle identities; PostgreSQL зберігає
regular_enabled для кожної historical assignment, тому replay старої idle
ревізії теж не надає права на regular results. Diagnostic observations окремо
перевіряються за ledger task. Міграція зберігає regular_enabled=true для старих
legacy assignments; старі легітимні результати лишаються доступні історії,
але не поточному assessment іншої ревізії.

Втрата обраного Agent не перепризначає checks іншому. Після TTL evidence
стає unknown/coverage_degraded; недоступність моніторингу не означає down
контрольованого ресурсу. Зміна policy запускає звичайне reconciliation.

```sh
netmonorc get --context-file /private/context.json PlacementPolicy
netmonorc plan --context-file /private/context.json -f resources.yaml
netmonorc apply --context-file /private/context.json -f resources.yaml --wait
netmonorc get --context-file /private/context.json agents
```

Критичні перевірки: mixed apply і JSONB round trip, залежна ревізія, заборона
видалення referenced policy, pending без partial, перевірка Agent identity,
HTTP request тільки від обраного scheduler, historical idle replay rejection.
Live Compose workflow у scripts/lab_placement.py включено в lab_sessions.py:
selected/idle, diagnostic на idle Agent, відключення обраного джерела,
pending без regular probes та restart. Фактичні результати запуску зберігаються
окремо у docs/experiments; наявність сценарію сама по собі не є його проходженням.

Для наступних вузьких ітерацій можна використати `lab_sessions.py --placement-only`
на новому owned stand: той самий штатний bootstrap/login та R24 use cases,
без повтору всіх попередніх workflows. PAT і cookie відкликаються наприкінці;
результат явно містить scope=fixed-placement-only. Повний режим лишається default.
