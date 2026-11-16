# Граф залежностей у веб-панелі (R28)

Користувач зі scope resources:read бачить секцію «Залежності ресурсів».
Панель читає наявний GET /api/v1/resources/Dependency; нових HTTP routes,
DSL-полів, бібліотек графів або алгоритмів оцінювання не додається.

Стрілка from → to означає «from залежить від to». Monitor і Target мають
різні підписи/кольори типу. Суцільна лінія означає requires, пунктир — інші
relation. Колір не показує health, strength не є severity. Підтвердження
несправності не виводиться з існування зв'язку.

Вибір вузла мишею, через список або клавішами Tab/Enter показує його вхідні
та вихідні зв'язки на завантаженій сторінці. Вибір ребра/назви в таблиці
відкриває повні from/to, relation, strength та description. Довгі імена
скорочуються лише на схемі: повні доступні в title, списку й таблиці.
RelatedTo cycles відображаються без спроби перетворити їх на requires DAG.
Валідація графа залишається на сервері.

Схема показує тільки поточну сторінку API (до 20 зв'язків), а не повний
каталог. Пошук і вибір вузла працюють у цих межах, що прямо зазначено в UI.
Кнопка наступної сторінки використовує opaque cursor сервера; «Оновити граф»
повертає першу сторінку. Знімок має час отримання й оновлюється за запитом.
Помилка читання прибирає старий граф; повторне оновлення відновлює його.
При зміні сторінки/виході pending fetch скасовується. Опис показується як
текст React, без HTML insertion. Редагування лишається в спільному editor.

Під час browser verification виправлено окремий дефект App: sibling keys
Overview/Configuration/Groups/Agents/Diagnostics/Alerts/Tokens/History/Users
повторювали userId. Щосекундне оновлення clock могло дублювати секції та
ремонтувати стан компонентів неправильно. Ключі тепер мають стабільний
префікс компонента й розділені user/Monitor identifiers. Новий graph теж
має окремий ключ. Формули, server validators та протокол Agent не змінені.

## Перевірка

```sh
npm run build --prefix web
node scripts/dependency_browser_smoke.mjs .local/browser-result.json .local/browser-panel.png
```

Потрібні Node 24+ і встановлений Chromium. Задавайте новий output JSON:
наявний файл не перезаписується. Script створює окремий тимчасовий browser
profile і loopback HTTP server; profile видаляється після завершення.
Chromium headless запущено без sandbox тільки для локальної fixture-перевірки.
Це не налаштування розгортання продукту. Сторонні npm packages не додаються.

Перевіряються реальні DOM/keyboard interactions production React assets:
стабільність секцій/вибору через clock ticks, напрямки стрілок, вхідні/вихідні
зв'язки, relation/strength, text escaping, current-page search, pagination,
503→clear→manual recovery, empty state та scope visibility.
Backend у цьому сценарії — read-only fixture; credentials не використовуються,
mutating requests заборонені. Це не перевірка auth/RBAC enforcement сервера,
TLS/PG/Agent integration або browser clicks проти live Controller. Останній
live dependency workflow збережено окремо в dependencies-r27.
