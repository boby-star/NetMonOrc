# Декларативні залежності (R27)

Окремий ресурс Dependency підключено до registry, REST CRUD/history/status,
mixed apply, CLI та конфігураційного редактора React. Залежність — це контекст,
а не Observation, доказ причини несправності чи команда на Agent.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Target
metadata: {name: corporate-dns}
spec: {type: logical}
---
apiVersion: netmonorc.io/v1alpha1
kind: Dependency
metadata: {name: web-requires-dns}
spec:
  from: {kind: Monitor, name: research-web}
  to: {kind: Target, name: corporate-dns}
  relation: requires
  strength: hard
  description: Вебсервіс залежить від корпоративного resolver
```

Monitor research-web повинен існувати або бути в тому самому mixed apply.
Target logical у прикладі описовий: власних перевірок і health він не набуває.
From → to означає «from залежить від to». Дозволені тільки Target/Monitor refs.
Defaults: relation=requires, strength=soft; description до 1024 символів.
Самопосилання заборонені для всіх relation. Валідація відхиляє dangling refs,
невідомі поля/enum та цикли requires до запису будь-якої частини пакета.

Граф розріджений. Kahn traversal перевіряє requires DAG без рекурсії за глибиною
мережі. Для routesThrough/hostedOn/resolvesVia/relatedTo цикли дозволені;
контекст обходиться з visited set, тому вони не утворюють нескінченної рекурсії.
Strength hard/soft не переносить стан на сусіда і не стає severity автоматично.

## Ревізії та Agent

У Monitoring IR входять вихідні доступні зв'язки від Monitor і його явного
Target ref, включно з транзитивними зв'язками. Вони відсортовані за ім'ям
Dependency. Address equality не використовується для вгадування зв'язку з
inline target. Усі refs перевіряються за повним proposed catalog незалежно
від порядку документів. Загальний ліміт каталогу і compiled-plan frame budget
зберігаються; завеликий контекст явно відхиляється.

Dependency-only зміна перебудовує залежні effective revisions Monitor та груп,
не змінюючи їхні source manifest/resourceVersion. Unrelated graph edge не
змінює Monitor, з якого він недосяжний. Apply дочікується implicit Monitor ACK;
сам Dependency має reason dependency_catalogued, що не означає healthy.
Порожній context не змінює старі default revisions. Agent перевіряє типи й
DAG у compiled IR; transport вимагає capability context:dependencies. Старий
Agent не може підтвердити незнайомий план. Controller та Agent оновлюються разом.

Видалення Target/Monitor, на який посилається Dependency, дає 409. Спершу
оновіть або видаліть зв'язок. Видалення Dependency прибирає IR context та
запускає звичайне reconciliation. CRUD успадковує RBAC/scopes, CSRF,
ETag/If-Match, audit/history та idempotency каталогу. Нових HTTP routes немає.

```sh
netmonorc get --context-file /private/context.json Dependency
netmonorc plan --context-file /private/context.json -f dependencies.yaml
netmonorc apply --context-file /private/context.json -f dependencies.yaml --wait
```

У React оберіть Dependency у конфігураційному редакторі. З R28 доступний
[інтерактивний перегляд графа](dependency-graph.md) поточної сторінки каталогу. Diagnostic Planner зберігає чинні hypotheses/Gain/Cost:
він не відсіює класи за здогадкою з Dependency. Доступність graph context в IR
не видається за реалізований автоматичний RCA.

## Короткий запис (додано 22.11.2026)

Monitor.spec.dependencies реалізовано за конкретним правилом DSL §15.1:
Monitor → Target, requires/soft. Явний requires має пріоритет для тієї самої
пари; спільний DAG, hash, delete guards і API/CLI/React context описані в
[актуальному контракті](monitor-dependencies.md). Формулювання §4.1 про
inline/linked target не використовується для створення прихованих Target.

Критичні перевірки: requires/non-requires cycles, unknown refs/self refs,
JSONB defaults, атомарна відмова cycle, delete guards, optimistic locking,
implicit revisions і Agent VerifyPlan. Вузький live сценарій
lab_sessions.py --dependencies-only використовує свіжий owned stand зі штатним
login; його результат додається окремо після виконання. DOM clicks не тестує.
