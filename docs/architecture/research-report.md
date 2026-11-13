# Приватний звіт для демонстрації

HTML-звіт показує вже обчислені Controller дані одного Monitor: зональні
стани, H/Q, coverage, reason codes, ID доказів та останню діагностику.
Таблиця діагностики містить дію, Agent, Gain/Score та execution/probe outcome.
Зарезервований бюджет не видається за кількість завершених фізичних probes.

Для робочого Controller додайте `-report-file .local/report.html`.
Файл атомарно оновлюється разом із Assessment; браузер перечитує його
кожні 5 секунд. Відкрийте файл на машині, де він зберігається, або скопіюйте
його для демонстрації. Скопійований звіт є snapshot і не отримує нових даних.

Завчасно збережені результати можна показати окремо:

```bash
go build -o .local/netmonorc-report ./cmd/netmonorc-report
.local/netmonorc-report -assessment .local/assessment.json -diagnostic .local/diagnostic.json -output .local/report.html
```

Параметр diagnostic необов'язковий; за відсутності сесій таблиця пояснює,
що результати ще не збережені. Чужий Monitor у diagnostic файлі відхиляється.
Попередня revision явно позначається як історія. Усі сесії, включно з тією
самою revision, є історичним поясненням; актуальний стан показано окремо.

Математична логіка залишається на Controller. Browser лише рахує вік
оцінки: понад 10s або ненадійний час показує попередження про застарілі дані.
Це freshness локального report, не заміна TTL окремих probes. Без JavaScript
попередження залишається видимим. H/Q тут не трактуються як ймовірності.

Файли створюються з mode 0600; користувацькі тексти екрануються html/template.
Зовнішніх скриптів, шрифтів, cookies чи мережевих запитів у звіті немає.
Додатковий HTTP listener не відкривається. Це read-only дослідницький звіт,
не повна React-панель чи реалізація REST API; адміністративних операцій немає.
Розповсюджуючи файл, врахуйте, що він містить назви ресурсів, зон та докази.

Перевірка: `scripts/distributed_smoke.py --diagnostics fixed` перевіряє
створення приватного файлу після реального Assessment. Unit-перевірка
зосереджена на екрануванні HTML і відмежуванні історичної revision.

Якщо Controller налаштований на алертинг, звіт показує status правила,
pending і час останнього enqueue для firing/resolved. Enqueue не означає
отримання webhook. Окремий CLI приймає `-alert .local/alert.json` разом
із assessment; без цього параметра додаткового блоку немає.

Controller також показує фактичний стан webhook outbox: чергу, leased,
delivered, failed, cancelled, останній event ID, attempts і HTTP status.
Це окремий read-only snapshot PostgreSQL лише поточного Monitor/rule/endpoint;
постановка в чергу не стає delivered. netmonorc-report із локальних snapshots
не має доступу до цієї проєкції; експортуйте готовий HTML Controller.
