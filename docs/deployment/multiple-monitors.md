# Кілька Monitor на одному Controller

Controller завантажує всі Monitor із persisted catalog разом із залежностями.
Стартовий `-config` залишається bootstrap-ресурсом; додаткові Monitor створюються
через звичайний resource editor, REST CRUD або CLI apply. Нових API routes,
DSL fields чи додаткових Controller-процесів немає.

Для кожного Monitor працюють окремі plan/revision, Assessment, діагностичний
координатор та cache alert evaluators. Incident budgets зберігаються в SQL
за Monitor. Один webhook delivery worker обслуговує спільний durable outbox.
Dashboard підраховує актуальні стани всіх завантажених Monitor; ResourceGroup
використовує їхні окремі оцінки. Просте збереження Monitor ще не означає
отримання свіжих результатів від Agent.

## Створення та перегляд

1. Запустіть [дослідницький стенд](quickstart.md).
2. У конфігураційному редакторі створіть ще один Monitor із новим metadata.name.
3. Для Monitor без explicit placement потрібні ті самі fixed roster zones, що
   й у bootstrap Monitor. Для дозволеної підмножини Agent використовуйте чинний
   fixed PlacementPolicy. Невідомі зони відхиляються під час validation.
4. У верхньому полі Monitor введіть його ім'я й натисніть **Відкрити Monitor**.
   Assessment, history, diagnostics та alerts перемикаються на вибраний ресурс.
   Ім'я зберігається в sessionStorage цієї вкладки; пароль туди не записується.
5. Для видалення додаткового Monitor використовуйте resource editor із його
   актуальною версією. Bootstrap Monitor, як і раніше, захищений від видалення.

Наявні `/api/v1/monitors/{monitorId}/assessment` та `/observations` обслуговують
кожний завантажений Monitor. Захист авторизації, freshness і effective revision
залишається на сервері. Невідомий Monitor не підміняється стартовим.

## Виконання та доставка

Один Agent підтримує одне mTLS-з'єднання та отримує послідовність незалежних
ConfigAssignment. Capability `plans:multiple` обов'язкова, якщо планів більше
одного: старий Agent відхиляється явно, а не перезаписує плани мовчки.
Кожна assignment має власні revision/checksum/lease й окремий ConfigApplied.
ACK записуються за revision у межах поточної session generation.
AgentView.appliedRevision лишається проєкцією bootstrap Monitor; окремі
підтвердження інших Monitor видно через apply operations. Новий ACK
не анулює попередні; reconnect вимагає підтвердження планів у новій generation.

Agent зберігає active plans за Monitor. Інтервали checks із однаковими ID
розділені за revision. Регулярні probes виконуються одним bounded worker;
порядок Monitor циклічно змінюється. Діагностика має окремий один worker.
Кеш містить не більше двох regular Blackbox Exporter процесів; за потреби
використовується витіснення. Це обмежує пам'ять, але для великого каталогу
додає вартість запусків exporter. Це не твердження про тисячі Monitor/секунду.

Spool/ACK підтримують batches із результатами різних Monitor. Поточна ревізія
потребує свого ACK; історичний replay перевіряється за persisted assignment.
Старі дані зберігаються в history і не визначають live assessment іншого Monitor.
Lease кожного плану незалежний. Видалений план більше не поновлюється;
Agent може виконувати його до завершення вже виданого lease. Після перезапуску
Agent збережені assignment files не відновлюють право запуску без нового lease.

Стартові JSON/HTML snapshot filenames збережені. Додаткові файли мають суфікс
імені Monitor перед розширенням: `assessment.research-other.json`, аналогічно
для diagnostic/alert/report. Поточний lab export/package збирає лише стартовий
демонстраційний набір; інші результати доступні через REST/history.

## Межі реалізації

Зміни Monitor catalog або effective revisions викликають спільний runtime
reconciliation із reconnect Agent streams. Безперервне незалежне оновлення
одного плану без перепідключення інших ще не реалізовано. Звичайні ACK і probes
не переривають сусідні плани. Серіалізований regular worker не гарантує
виконання всіх інтервалів за перевантаження: відсутні свіжі докази дають
недостатнє coverage/unknown, а не фіктивне healthy.

Dynamic placement, replica voting та Agent enrollment/rotation залишаються
окремими незавершеними блоками. Partial placement лишається вимкненим.
