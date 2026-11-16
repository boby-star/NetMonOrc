# ADR 0011: незалежні Monitor plans у спільній Agent session

Дата: 2026-11-16. Рішення реалізації в межах погоджених v1alpha1 contracts.

## Проблема

Каталог уже підтримує кілька Monitor, але Controller/Agent виконували тільки
bootstrap plan. Один applied_revision на Agent не може підтверджувати два
одночасні плани: останній ACK витісняє попередній.

## Рішення

Controller завантажує узгоджений compiled catalog, створює окремий runtime
кожного Monitor і передає плани через чинні ConfigAssignment в одному stream.
Нових protobuf fields, HTTP routes, YAML resources чи математичних правил немає.
Capability plans:multiple захищає від мовчазного приймання старими Agents.

Session generation залишається спільною для фізичного Agent. Таблиця
nmo_session_revisions містить окремі ACK revision/generation; session row lock
захищає їх від reconnect. Legacy applied_revision збережений для сумісності
схеми, але нова авторизація його не використовує. Перевірки diagnostic tasks
і config.apply readiness також використовують per-revision ACK.

Agent має map Monitor → assignment з власним lease; один regular worker,
один diagnostic worker та bounded cache двох regular exporters. Не створюємо
worker або exporter на кожний Monitor. SQL Assessment, incident budgets,
alert evaluators і runtime snapshots розділені за Monitor.

## Наслідки та межі

REST/React можуть переглядати кожний Monitor; summary враховує всі актуальні
завантажені оцінки. Поточний catalog reconciliation ще перепідключає спільні
streams; незалежний hot reload потребує окремої наступної зміни. Видалення
припиняє renew, а вже виданий lease закінчується за чинним expiresAt. Старі
observations лишаються історичними. У повільному/перевантаженому стенді freshness
чесно зменшується; необмеженого parallelism не додаємо.

Схема PostgreSQL змінюється адитивно. Після оновлення Controller старі ACK
не переносяться автоматично: Agent підтверджує плани у новій session generation.
