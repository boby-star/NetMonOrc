# Пакетне застосування та операції (R7)

POST `/api/v1/config/apply` зберігає набір 1..20 підтриманих Monitor однією
PostgreSQL-транзакцією. Помилка версії або інший конфлікт відкатує весь набір,
ledger і створення Operation. Ресурси поза пакетом залишаються; implicit prune
немає. Потрібні admin+resources:write, чинні credentials, CSRF для cookie.

ConfigBatch містить manifests, expectedVersions та необов'язковий requestId.
Для наявного Monitor задайте поточний resourceVersion за ключем Monitor/name.
Пропущена precondition наявного ресурсу — 428, застаріла — 412. Null або
відсутня precondition для нового ресурсу означає create-only; наявна назва
дає 409. Не передавайте ETag quotes усередині expectedVersions. Зайві ключі
версій не приймаються. Повна перевірка compiler/CEL залишається на сервері.

## Повтор після втрати відповіді

requestId діє як idempotency key в межах user ID протягом 24 годин. Це
ідентичність користувача, а не конкретна browser session/PAT. Повтор того самого
canonical typed JSON body повертає ту саму збережену Operation без нового
upsert, навіть якщо її preconditions вже застаріли після першого commit.
Поточні credentials/права перевіряються і під час recovery. Інший body із
тим самим ключем дає 409 IDEMPOTENCY_KEY_REUSED. Порядок ключів JSON/whitespace
не має значення; порядок manifests і числове JSON-представлення враховуються.

Після завершення 24 годин ключ може використовуватися заново, але звичайні
preconditions перевіряються знову. TTL idempotency не є timeout виконання і
не видаляє історію Operation. Якщо requestId не заданий, сервер не гарантує
recovery повторного POST; панель завжди формує його до відправлення запиту.

## Стани

202 Accepted та Location вказують на GET `/api/v1/operations/{id}`.

| Стан | Що він підтверджує |
|---|---|
| accepted | Бажана конфігурація збережена атомарно |
| running | Controller завантажує/узгоджує релевантні assignments |
| completed | Усі потрібні агенти підтвердили актуальний план для всіх targets |
| failed | Target видалено або замінено до завершення, CONFIG_SUPERSEDED |

Стан не визначає healthy/down мережевого сервісу. Для ready використовується
поточний stream ACK, heartbeat не старший 15s та повторна перевірка session
generation/applied_revision у БД. Старий stream після reconnect не підтверджує
нову Operation. Completed — збережений факт завершення; наступна втрата агента
не переписує історичну операцію, а відображається coverage/Assessment.

Controller обробляє до 10 pending operations за цикл, ротуючи їх за updated_at.
Проблема progress reconciliation відкладає оновлення операцій, не зупиняючи
regular assessment. Operation та idempotency переживають restart Controller.
Незавершена операція не перезаписує новіші desired resources назад.

Поточний Controller виконує один active Monitor. Невибраний Monitor у пакеті
залишається pending; пакет із ним не позначається completed автоматично.
Result.targets показує ready/reason кожного елемента. Multi-Monitor execution,
інші DSL kind і general placement не додаються неявно цим endpoint.

GET `/api/v1/operations` потребує resources:read, підтримує limit 1..200,
ID DESC/cursor та state. GET конкретної Operation також потребує resources:read.
Це наявні маршрути OpenAPI. Credential hashes/request body hash не повертаються.

## Панель

У «Конфігурація Monitor» відкрийте «Пакетне застосування й операції».
Підготуйте пакет із чернетки відкритого маніфесту: панель додає відому версію
ресурсу та випадковий requestId. JSON можна доповнити іншими Monitor/versions.
«Перевірити пакет» не змінює desired. «Застосувати пакет» показує Operation та
опитує її стан; список і фільтр операцій завантажуються за запитом користувача.
Якщо відповідь втрачена, повторіть незмінний пакет. Для іншого запиту створіть
новий requestId. Після apply перечитайте ресурс перед звичайним PUT.

Перевірено один критичний PostgreSQL/HTTPS сценарій rollback/replay/fencing
та один Compose сценарій apply→physical Agent ACK→completed, process restart,
idempotent replay і відновлення початкової конфігурації. React production
build пройшов; browser DOM clicks не автоматизовані.
