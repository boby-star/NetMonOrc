# Завершення replicas та CEL-контексту діагностики

Реалізовано replicas 1–10 для DiagnosticAction у Fixed, Adaptive та режимі
порівняння Static. PlacementRef та inline використовують один compiler.
За відсутності action placement береться policy поточної ревізії Monitor;
reusable profile source/hash не переписується під конкретний Monitor.
Успадковуються candidate refs, strategy, replicas, zones і distinctBy. Pending
не перетворюється на дозвіл запускати probe на довільному Agent.

## Планування та облік

Перед першим запуском обирається повний набір N різних Agent з урахуванням
trusted selectors, refs/zones, capabilities, eligibility, distinctBy та
мінімального бюджету N. Missing sources не зменшують N. Якщо бюджету для
першого набору не вистачає — жодного probe; причина
check_budget_insufficient_for_replicas. Частковий onInsufficientAgents=partial
не доданий: це окрема, раніше вимкнена семантика regular placement.

Кожний фізичний task має власний ID/Agent/Observation, durable count/byte
reservation і retry. Крок DAG має спільний stepId, але кілька task IDs.
Retry лишається на своєму Agent, витрачає ще одну спробу й не дає refunds.
Скасування, deadline, втрата source або витрати на retries можуть залишити
частину вже запущеного набору: вона зберігається в історії, а незавершений
Fixed крок не оголошується completed. Parallel limit залишається верхньою
межею; за maxParallel=1 replicas виконуються послідовно.

Fixed завершує крок лише після terminal results усіх його replicas, а потім
відкриває залежні кроки. Adaptive вибирає дію за gain/(cost × N), виконує весь
вибраний набір до наступного логічного вибору й враховує кожне фізичне
спостереження окремо. Рання унікальність після першої replica не скасовує
решту вибраного набору. Конфліктні факти не стають більшістю/здогадкою:
залишаються evidence_conflict/inconclusive. Baseline може дати висновок без
вибору нової дії; це не рахується як виконані replicas цієї дії.

## Результати replicas у Fixed CEL

```cel
tasks.read.replicaResults.branch.values.depth == 0 &&
tasks.read.replicaResults.dc.values.depth == 0
```

replicaResults — map за фактичним Agent ID. Кожний елемент має values,
reasonCode, outcome/executionStatus, agentId, zone, failureDomain,
observationId. Існуючі scalar types та TTL/provenance guards збережено.
Для dynamic placement використовуйте map macros, а не очікування довільного
конкретного Agent ID.

Top-level outcome — unanimous success/failure, інакше unknown; executionStatus
completed лише за завершених процедур усіх replicas. Top-level values для
N>1 порожні: числової агрегації немає. Reason code узагальнюється лише за
збігу, інакше replica_results_diverge. Raw physical results не підміняються
цим summary. Незалежні DAG-гілки не доступні через tasks.

## Повний діагностичний CEL-контекст

| Контекст | Джерело та поля |
|---|---|
| monitor | name/state/coverageState, labels Monitor, optional weightedScore |
| target | address/type/networkZone/criticality, labels та typed attributes Target ref |
| tasks | outcomes/status/values/reasonCode попередників, replicaResults та physical provenance |
| diagnostic | attempts, remainingChecks, elapsedSeconds — фактичні ledger/session counters |
| checks, zone | Свіжі придатні check views і вже оцінений стан однієї однозначної зони дії |
| zones["office"] | Явний контекст зони: name/state/coverage/labels/checks/optional weightedScore |

Контекст regular checks перебудовується з зафіксованого input snapshot за
поточним часом через той самий Assessment Engine: revision/TTL/maxSkew,
planned replicas та required gates не дублюються приблизними правилами.
Нові діагностичні результати не підставляються як regular observations.
Bare checks/zone відсутні, якщо placement допускає кілька зон; звертайтеся
до zones["name"].checks. Не обирається випадкова перша зона.

weightedScore Monitor є H, коли зона одна і H відома. Для кількох зон DSL
не задає ваги для одного числового score: поле відсутнє, доступне через has,
а зональні H — через zones[name].weightedScore. Середнє не вигадується.
Zone labels — відомий порожній набір, доки немає джерела zone inventory;
вони не копіюються із самоопису Agent. Доступність конкретної metrics key
залежить від реального probe; наявність map не створює latency вимірювання.

Trigger також отримує monitor labels/optional score і target attributes.
Невідомі static fields відхиляються до запису. Fixed check/zone refs
перевіряються при binding до Monitor, зокрема zones[name].checks; manual
start перевіряє те саме. Source labels та Target attributes входять в
ревізійно зв'язаний контекст, тому зміна metadata не залишає старі докази live.
Ліміти Trigger/Fixed — 4096 bytes, cost=1024; runtime не виконує I/O з CEL.

[Повний YAML](../../examples/diagnostics/replicated-context.yaml) призначений
для приватного fixture 198.18.0.80 з branch/office та dc/server. CLI validate
використовує той самий catalog resolver. REST та generic React YAML editor
працюють із цими ж manifests, без frontend diagnosis calculation.
[Натурні докази](../experiments/replica-context-2026-11-23/README.md).

Це завершення вибраних діагностичних функцій, не сертифікація всього старого
ТЗ: Alert zone scope, regular multi-replica values aggregation та partial
placement — окремі обмеження. Нових browser clicks у цьому прийманні немає.
