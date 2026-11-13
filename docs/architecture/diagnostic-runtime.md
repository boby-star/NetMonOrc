# Операційний контур діагностики дослідницького прототипу

Controller викликає те саме чисте ядро Fixed/Adaptive після Assessment.
Прапорець `-diagnostic-comparison-static` запускає Static baseline з того
самого профілю, каталогу, Agent і бюджетів. Значення mode: static у YAML
немає; вибір baseline входить до effective revision як внутрішній IR context.
Стандартні probe drivers і дозволені мережеві цілі Agent залишаються спільними
для регулярних перевірок і діагностики. Нових REST routes або YAML enums немає.
Це обмежений дослідницький контур; повний agent protocol §9 не завершено.

## Зв'язування

До `spec` Monitor додається чинний DiagnosticBinding:

```yaml
diagnostics:
  enabled: true
  profileRef: live-web
  onStates: [degraded, down]
  when: 'monitor.coverageState != "unknown"'
  cooldown: 5m
```

Controller отримує файл профілю через `-diagnostic-profile`; його name має
збігатися з profileRef. Наявність файлу при disabled binding відхиляється.
Hash нормалізованого профілю, включно з Fixed CEL expressions, входить до
hash ефективної конфігурації. Інші profile refs не реалізовані.

Trigger підтримує monitor.name/state/coverageState з bool type-check,
1024 cost units, 2048 bytes expression і deadline 25ms. Невизначений trigger
нічого не запускає. Інші CEL monitor fields цим профілем не надаються.
Приклади: `examples/diagnostics/live-{adaptive,fixed}.yaml`.

## Доставка та облік

1. Автентифікований Agent підтверджує revision та передає heartbeat:
   maxConcurrentProbes=2, activeProbes як поточний gauge, sequence в межах
   session generation. Без свіжого heartbeat місткість невідома.
2. Для Monitor створюється durable incident/session. Перед відправленням
   кожного Task PostgreSQL атомарно резервує один check у бюджеті.
3. Agent звіряє revision, enabled binding, lease, deadline, типізований check
   і локальну CIDR policy. Receipt зберігається до запуску, повторний task ID
   не запускається. Один regular slot та один diagnostic slot незалежні.
4. Результат має diagnosticTaskID і потрапляє до тієї самої bounded spool.
   Місце резервується перед кожним probe, тому паралельні lanes не забирають
   одна в одної останнє місце. Видалення — лише після durable ACK.
5. Controller перевіряє agent/zone/check/revision проти task reservation.
   Такі observations зберігаються, але виключені з live Assessment і baseline
   matching. Gain/Cost, outcome і вибраний Agent записуються у звіт.

У цій підмножині результат задачі передається ObservationBatch з task ID,
а не окремим TaskResult frame. TaskAccepted/Rejected — типізований TaskReceipt.
Це явно описане обмеження agent wire protocol, не новий публічний API.

## Відмова й перезапуск

Один процес Controller утримує PostgreSQL advisory lock. Generation fencing
для Agent діє й під час task result commit. Після crash попередні running
sessions стають inconclusive/controller_restart; задача не перепризначається.
Пізній результат зберігається як історичний і не відновлює завершену сесію.
Локальний звіт також відновлюється з БД, щоб не показувати попередній успішний
діагноз замість перерваного запуску. reservedChecks відділено від відомих
outcomes: reservation не є доказом фізичного завершення probe.

Поточне спрощення — **одна сесія на епізод**. Нові observations, reconnect,
revision чи завершення сесії не відкривають другу. Лише актуальний healthy
з повним coverage закриває епізод; cooldown зберігається й після закриття.
Невідправлені/відхилені reservations консервативно витрачають бюджет.
Це не повна реалізація багатосеансової діагностики з confirmation/retry.

Clock uncertainty для стенда прийнята рівною 5s. Agent віднімає її від
абсолютного task deadline і обмежує його lease. Тому операційний профіль
вимагає timeout щонайменше 6s; приклади використовують 10s. Тривалий probe
може завершитись раніше як execution_failed, якщо вичерпано безпечний час.
Ця межа не є результатом вимірювання синхронізації годинників.

## Перевірка

```bash
go build -o .local/netmonorc-controller ./cmd/netmonorc-controller
go build -o .local/netmonorc-agent ./cmd/netmonorc-agent
python3 scripts/distributed_smoke.py --diagnostics fixed
python3 scripts/distributed_smoke.py --diagnostics adaptive
python3 scripts/distributed_smoke.py --diagnostics static
python3 scripts/distributed_smoke.py --diagnostics adaptive --diagnostic-runner blackbox --blackbox-binary /path/to/blackbox_exporter
```

Сценарій перевіряє physical probes, durable budget, restart без повтору,
kill активного Controller, inconclusive після crash, новий епізод лише після
healthy/full coverage, replay, TTL і ізоляцію результатів у PostgreSQL.
Це кілька процесів на одному хості. Висновків про якість локалізації між
незалежними мережевими сегментами або перевагу над Static ці дані не дають.

Не реалізовано public/manual start, TaskCancel, technical retry, replicas,
паралельний planner, byte estimator, повний placement/PKI та натурні мережеві
експерименти. Без byte estimator maxEstimatedBytes для live профілю
відхиляється. Closed-catalog early-stop limitation описано в diagnostic-core.md.

## Вимірювання часу

`result.durationMs` вимірюється монотонним годинником усередині Engine:
від початку Run до повернення результату, включно з task dispatch, probes
і очікуванням durable result. Затримка між появою відмови та trigger сюди
не входить. Після аварії втрачене вимірювання не відновлюється як нуль:
поле відсутнє, HTML показує «—». У replay це час синтетичного обчислення,
а не мережевої діагностики. Diagnostic Blackbox запускається на кожний task;
його startup входить у час. Ця витрата однакова для runner-а всіх режимів.

Live smoke Static з Blackbox пройшов обидва tasks спільного каталогу,
класифікацію service-failure, збереження budget і crash/recovery. Це лише
інтеграційний сценарій, не контрольований статистичний benchmark.

## Явне підтвердження

Controller виконує adaptive.confirmation у тому самому task transport,
spool та durable budget. Процедура використовує обов'язкові actionRefs,
minObservations, minDistinctFailureDomains, minSpacing. У звіті є окрема
phase confirmation та лічильники доказів/domain. Дані одного baseline
observationID не подвоюються через кілька aliases. Невідомий domain не
додає незалежного джерела; одна зона залишається одним path failure domain.

У Fixed технічна невдача або невідома CEL-умова активованого кроку не
дозволяє completed. Відомий when=false — штатний пропуск. Для live стенда
є explicit four-step fixed-confirmed DAG; original branching profile
залишається окремим legacy варіантом. Retry й відновлення багатосеансової
історії confirmation після crash не реалізовані: run лишається inconclusive
без повторного запуску й повернення reservations.
