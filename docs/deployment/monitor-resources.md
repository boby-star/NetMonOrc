# Monitor CRUD та узгодження конфігурації (R6)

Контролер має один вибраний Monitor і фіксований roster агентів. Реалізований
повний цикл для підтриманого Monitor: створення/читання/заміна/видалення,
історія версій, серверна перевірка й dry-run, збереження PostgreSQL і доставка
нового активного плану агентам. Інші kind та multi-Monitor execution залишаються
наступними етапами; сервер відхиляє їх явно, без збереження неперевіреного spec.

## Панель

Після парольного входу відкрийте «Конфігурація Monitor». Панель читає JSON-
подання того самого декларативного DSL. Адміністратор може редагувати його,
виконати «Перевірити», «Показати план» і «Зберегти поточний». Кнопка «Завантажити»
перечитує запис із сервера; вона замінює локальну чернетку актуальним manifest.
Перевірка умов, defaults і CEL лишається на сервері.

Для нового запису змініть metadata.name і натисніть «Створити новий Monitor».
Невибраний Monitor зберігається зі статусом pending; виконання не вигадується.
Видалення вибраного Controller Monitor повертає 409. Standby Monitor можна
видалити; історичний ledger залишається в БД, але GET revisions після видалення
відсутнього ресурсу дає 404. Повторне створення отримує інший uid.

Viewer може читати конфігурацію. Mutations та validate/plan потребують admin
і resources:write; читання — resources:read. Для cookie потрібні CSRF і
same-origin, для відповідного PAT — його scopes. Bootstrap reader лишається
обмеженим Assessment. Створення/зміна/видалення проходять повторну перевірку
чинного credential, ролі й scope у транзакції та пишуть audit без secret.

## Версії та API

| Запит | Призначення |
|---|---|
| GET/POST `/api/v1/resources/Monitor` | Список / create-only |
| GET/PUT/DELETE `/api/v1/resources/Monitor/{name}` | Читання / full replace / видалення |
| GET `/api/v1/resources/Monitor/{name}/revisions` | Історія поточного uid |
| GET `/api/v1/resources/Monitor/{name}/status` | Стан завантаження бажаного плану |
| POST `/api/v1/config/validate` | ValidationResult без змін |
| POST `/api/v1/config/plan` | Diff та effective revisions без змін |

GET/POST/PUT ресурсу повертають сильний ETag. PUT і DELETE вимагають цей ETag
у If-Match: без нього 428, застарілий — 412. Після 412 перечитайте ресурс і
перевірте чернетку; автоматичного перезапису немає. Body не приймає status,
uid/generation/resourceVersion; kind та name повинні відповідати URL.

uid — ідентичність об'єкта. resourceVersion — opaque версія кожного прийнятого
PUT, включно зі зміною metadata. generation збільшується лише за зміни spec.
Зміна metadata не змінює effective revision перевірок, якщо їхня нормалізована
логіка лишилася тією самою. ResourceRevision містить manifest, generation,
resourceVersion, timestamp та actor. Ledger прив'язаний до uid, тому історія
попереднього видаленого об'єкта не змішується з повторно створеним.

Список: limit 1..200, default 50; name ASC, cursor останнього name.
labelSelector — AND пар key=value через кому. Revisions: новіші version
першими, nextCursor передається без розбору. Фільтр state не підтриманий і
дає 400. Це live pages, не snapshot всіх сторінок.

Validate/plan приймають JSON ConfigBatch з 1..20 manifests, body ≤1 MiB.
Registry запускає наявний суворий compiler, CEL і resolver доступного
DiagnosticProfile, без мережевих probes. Validate повертає valid=false/errors
для некоректної конфігурації. Plan повертає create/replace/unchanged,
поточний resourceVersion, effective revision та deterministic planId.
placements порожній із warning про fixed roster; plan не резервує виконання
й не створює асинхронної операції. Batch apply/operations поки не реалізовані.

## Як новий план доходить до агентів

На першому запуску Controller імпортує Monitor з --config у PostgreSQL.
Наступні старти з тим самим ім'ям читають збережений desired state; bootstrap-
файл не перезаписує його. Файл залишається потрібним як сумісний bootstrap і
вибір імені Monitor, зон та зовнішніх flags. DiagnosticProfile, AlertRule і
WebhookEndpoint у цьому зрізі ще завантажуються з наявних локальних файлів.

Controller щосекунди перевіряє effective revision. Нова ревізія завершує старий
runtime через cancellation і graceful shutdown, очікує assessment/notification/
diagnostic workers та створює новий runtime у тому самому процесі. mTLS
Agents перепідключаються, отримують нову session generation і assignment lease.
Звичайний lease/replay/буфер результатів зберігає чинні гарантії. Поточні
діагностичні задачі завершуються за правилами cancellation; їхній incident
budget не скидається. User sessions і PAT залишаються в PostgreSQL.

HTTP Assessment читається одним SQL snapshot, який перевіряє одночасно:
результат належить завантаженому Controller плану та desired effective revision.
Після commit нового desired попередній healthy не повертається як live: до
актуального Assessment сервер дає 503. Після завантаження плану неповні дані
дають unknown за існуючим алгоритмом; нові спостереження визначають стан.
Старі результати лишаються історією та не заповнюють coverage нової ревізії.

ResourceStatus active означає лише завантажений план Controller; доступність
мережевого ресурсу показує окремий Assessment. Під час reconciliation HTTP
або gRPC можуть бути короткочасно недоступні; клієнтам потрібен повтор запиту.
Метадані без зміни execution IR не перезапускають перевірки.

## Поточні межі

Registry підтримує лише вже реалізований Monitor DSL-профіль. targetRef,
useProfiles, інші kind та регулярні replicas ще не додаються неявно. Зони
активного Monitor мають відповідати незмінному fixed roster. Увімкнена
діагностика потребує наявного зовнішнього DiagnosticProfile; зміни його
прив'язки повинні бути сумісні з flags Controller. Перехід enabled→disabled активного binding із заданим --diagnostic-profile
поки відхиляється: перемикання цього запускового режиму потребує окремого
узгодженого переходу bootstrap/flags/desired state, який у цьому зрізі не реалізований.

Валідація і планування не є повною реалізацією config/apply. Реальне застосування
цього зрізу виконується одиночним PUT активного Monitor. Загальні dependency
resources, multi-resource atomic apply, operations, placements і multi-Monitor
execution залишаються відкритими. Математичні правила Assessment не змінені.

Перевірки: `python3 scripts/identity_smoke.py` — критичний PostgreSQL/HTTPS
сценарій CRUD/version/generation, label filter, validation/history, PAT scopes,
ізоляція live revision. Окремий Compose scenario змінює expectedStatus через
REST, отримує новий down від фізичних Agent probes, перезапускає процес,
підтверджує persisted desired та відновлює healthy з іншою ревізією.
Browser DOM clicks не автоматизовані; TypeScript/Vite production build перевірений.
