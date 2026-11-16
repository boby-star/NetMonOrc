# ADR 0012: persisted динамічний regular placement

Дата: 2026-11-16. Реалізація наявних leastLoaded/roundRobin із ТЗ §4.2
та DSL §8; partial лишається вимкненим за рішенням користувача.

## Рішення

Feasible набір визначається trusted roster, policy filters, актуальною
автентифікованою session telemetry, capabilities та тією самою IP target
policy, яку застосовує Agent. Local allowlist передається через захищені
Hello capabilities; це не новий публічний YAML або protobuf field.

Вибір sticky: зберігати повний допустимий selection; обирати новий лише за
його відсутності/втрати придатності/зміни policy. Так миттєві коливання active
не створюють revision churn. leastLoaded — фактичне active/capacity із tie
agentID ASC. roundRobin — persistent cursor на policy UID або inline Monitor
UID, спільний між її новими призначеннями. Недостатній набір є pending.

Resolved selections зберігаються за Monitor UID і policy hash. Compile/load,
apply і placement reconciliation використовують один catalog transaction lock.
Selection входить у нормалізований execution plan та його effective hash;
математична логіка Assessment не змінюється. Historical assignment permission
залишається revision-scoped, як до цієї зміни.

Для config.apply записуються тільки фактичні placement-переходи effective
revisions. Operation може відстежити їх за незмінних UID/source version та
все одно потребує потрібних Agent ACK. Відновлення hashes може утворити цикл,
тому recursive SQL використовує UNION із дедуплікацією revisions. Зміни
конфігурації не утворюють таких edges і лишаються superseding changes.

Збережений selection додатково прив'язаний до hash повного розгорнутого
Monitor без selection. Зміна target, runner, checks чи залежних profiles
спочатку переводить його в pending і вимагає нового перевіреного відбору;
самої рівності policy hash недостатньо для повторного дозволу запусків.

## Компроміси

Runtime reload ще глобальний; 15-секундний initial grace дає новим sessions
передати heartbeat, а збережені selections відновлюються одразу. Незалежний hot
reload, selectors, failure-domain registry, same-zone replicas і rebalancing
не входять до цієї ітерації. Відсутність placement/ref зберігає legacy static
behavior стенда; explicit PlacementSpec уже використовує default leastLoaded.
