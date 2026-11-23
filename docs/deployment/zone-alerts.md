# Зональні AlertRule та вибір Monitor за labels

AlertRule підтримує `scope: monitor` (типово) і `scope: zone`. Прив'язка задається
рівно одним із `monitorRef` або `selector`. Селектор перевіряє metadata.labels
Monitor через matchLabels/matchExpressions, а не labels Agent. Порожній selector
вибирає всі Monitor; нуль збігів дозволений. [Повний приклад](../../examples/zone-alerts.yaml).

```sh
netmonorc validate --config examples/zone-alerts.yaml \
  --slots branch=office,dc=server --allow-cidrs 192.0.2.0/24
```

Це локальна валідація каталогу, без виконання мережевих запитів. У прикладі
webhook вимкнений, IP належать діапазону документації. У робочому середовищі
потрібні справжній HTTPS endpoint, довірений CA та дозволений CIDR.

## CEL і валідація

Обидва scope мають типізовані monitor (включно з labels/score), target
(із scalar attributes), zones[name] та checks. У zone scope додатково доступний
zone: name, state, coverage, checks, optional weightedScore. Bare checks тут
належить лише поточній зоні. У monitor scope bare checks визначені лише для
однієї зони; для кількох потрібне явне zones[name].checks. Monitor.state завжди
залишається загальним станом Monitor, навіть під час оцінювання окремої зони.
Zone labels поки порожні: довірений каталог labels мережевих зон не створюється.

За замовчуванням resolveWhen — zone.state == "healthy" для zone scope,
monitor.state == "healthy" для monitor scope. Unknown не є автоматичним
відновленням. Відсутні optional values не замінюються нулями; int64 зберігається
без округлення до float64. CEL має обмеження довжини 4096 та вартості 1024.
Невідомі типізовані поля, не-bool умови, статичні невідомі check/zone refs
відхиляються до запису. Перевіряються checks.*, zone.checks.* та
zones.*.checks.*, включно з літеральними індексами. Обчислювані ключі
залишаються runtime-перевіркою; помилка CEL не доводить recovery.

Спільний compiler перевіряє refs для кожного вибраного Monitor. Зміна labels,
яка додає Monitor із відсутнім check, також відхиляється. CLI validate, API
validate/plan, CRUD і атомарний apply використовують цей самий граф.

## Незалежні екземпляри та життєвий цикл

Кожна пара Monitor–зона має власний ключ rule/monitor/zone/zoneName, pending,
for, firing, cooldown, історію, ACK і webhook outbox. Старий monitor key
rule/monitor/monitor збережений. Для zone scope reason/evidence і payload.zone
беруться лише з відповідної зони; здорова сусідня зона не отримує чужий firing.
Durable доставки залишаються at-least-once зі сталим event ID.

Нові збіги selector починають оцінюватися автоматично. Вилучення Monitor із
вибірки блокує нові firing, скидає pending, але не підміняє фактичне відновлення
вже firing екземпляра. Такий екземпляр далі перевіряє resolveWhen; після recovery
повторний збій невибраного Monitor не запускає новий цикл. На новому циклі ACK
попереднього циклу очищається; під час recovery ACK зберігається в історії.

Зміна scope/monitorRef/selector та DELETE правила з pending/firing екземплярами
повертають 409. Видалення Monitor із такими активними екземплярами теж захищене,
включно із selector-based правилами. Це не забороняє зміну when/resolveWhen або
enabled і не вигадує resolve під час вимкнення. Зміна labels Monitor дозволена.
Фактична membership перевіряється під тим самим транзакційним catalog lock,
що й UID/hash правила, effective revision Monitor та запис FSM/outbox.

Apply --wait очікує оцінювання поточного rule hash після початку операції на
поточній ревізії кожного вибраного Monitor і кожної required zone. Нуль збігів
завершується reason alert_selector_empty, що означає збереження без runtime
оцінювання; звичайне підтвердження має reason alert_rule_evaluated.

React Alerts показує зону, фільтр за зоною, окремі details/ACK/history/evidence.
Загальний локальний snapshot/report продовжує показувати один підсумковий стан
першого правила; повний перелік екземплярів доступний через API/React.
Зміни схеми БД для цього не потрібні. Повний UID-based lifecycle повторного
створення правил і поведінка після вилучення required zone — окремі питання;
цей етап не оголошує їх реалізованими.

## Перевірка

scripts/lab_zone_alerts.py створює власний PostgreSQL/Controller/двох mTLS Agents,
реальний HTTP target та HTTPS webhook. Сценарій перевіряє apply readiness,
селектори, локальну несправність office при healthy server, evidence, ACK,
409 guards, recovery вилученого з вибірки Monitor і новий цикл без старого ACK.
Команда down зберігає .local, PKI і volumes. Дані доступу не входять у звіт.
Фіксований HTTPS/Docker стенд не є доказом browser clicks або роботи в різних
фізичних мережах. [Фактичні результати](../experiments/zone-alerts-2026-11-23/README.md).
