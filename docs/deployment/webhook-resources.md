# Збережені WebhookEndpoint (R20)

WebhookEndpoint підключено до наявного REST каталогу ресурсів, validate/plan/apply,
CLI та React-редактора. Використовується чинний DSL: url, enabled, auth.type,
auth.secretRef, timeout та retry. Нові YAML-поля чи API-маршрути не додано.

## Один активний канал

Застосовується DSL §14: не більше одного enabled=true endpoint. Неактивні
ресурси дозволені для підготовки заміни. У змішаному apply можна одночасно
вимкнути старий канал, увімкнути новий та змінити AlertRule.channelRef.
Перевіряється весь кінцевий граф, незалежно від порядку документів.
Enabled AlertRule повинен мати наявний channelRef; посилання на неактивний
канал оцінює правило, але не надсилає повідомлення через цей канал.
Endpoint, на який посилається enabled правило, захищено від видалення (409).

```sh
netmonorc get --context-file /private/context.json WebhookEndpoint
netmonorc get --context-file /private/context.json WebhookEndpoint default-webhook
netmonorc plan --context-file /private/context.json -f channels.json
netmonorc apply --context-file /private/context.json -f channels.json --wait
```

У React виберіть WebhookEndpoint у «Конфігурація моніторингу». Доступні каталог,
читання, validate/plan, створення, заміна, видалення та mixed Batch Apply.
GET потребує resources:read; mutations — admin + resources:write, cookie CSRF,
а для replace/delete — If-Match. Використовуються спільні ETag, history,
generation/resourceVersion, audit та 24-годинна idempotency.

## Мережа й credentials

Компілятор перевіряє HTTPS, IP-literal URL, заборонені metadata/link-local адреси,
допустимі timeout/retry та явний --webhook-allow-cidrs. CIDR policy, CA bundle
і каталог секретів задаються оператором поза DSL. Через API вони не змінюються.
Redirects заборонені, TLS verification увімкнена, auth використовує локальний
secretRef. API зберігає лише посилання, а не вміст секрету.

Перед commit активної конфігурації infrastructure preflight створює й одразу
закриває Sender: перевіряє читання private non-symlink secret file та CA/TLS
налаштувань без HTTP-запиту. Помилка дає 422 і відкочує весь batch. Неактивну
заміщувальну конфігурацію можна підготувати до встановлення її секрету;
при активації preflight обов'язковий. Validate/plan не підтверджують наявності
локальних credentials — остаточну перевірку виконує mutation transaction.
Компілятор не читає файли й не виконує мережевих запитів.

## Завантаження runtime

Startup --webhook-config залишається початковим імпортом; парні --alert-rule
і --webhook-config налаштовують subsystem. Існуючий desired запис має пріоритет
над файлом. Видалення записує bootstrap marker і не відновлює його після restart.
Controller вибирає єдиний активний endpoint із PostgreSQL та створює Sender.
Якщо активних немає, доставка вимкнена. Неактивні канали доступні для оцінювання
правил без enqueue notifications.

Зміна UID або нормалізованого hash будь-якого endpoint запускає контрольоване
перезавантаження runtime. HTTP і mTLS streams закриваються в порядку штатного
завершення, Agents перепідключаються з generation fencing; користувацькі сесії
залишаються в PostgreSQL. Зміна labels без зміни hash не потребує reload.
Заміна вмісту локального secret file/CA потребує перезапуску Controller.

CLI apply повторює незмінний пакет зі сталим requestId після тимчасового
обриву з'єднання або 429/503 в межах заданого timeout. Серверна idempotency
повертає ту саму operation після втрати відповіді. TLS trust/validation,
авторизація та precondition errors не обходяться повторним запитом.

Apply target ready означає, що чинний Controller завантажив відповідні UID/hash
і успішно створив Sender для активного каналу. Тільки commit desired не завершує
операцію. Це не підтвердження доступності одержувача: перевірка доставки —
окремий webhook test, completed якого означає HTTP 2xx.

Оцінювання алерта, queue webhook test та claim outbox перевіряють чинний hash
endpoint у транзакції зі спільним catalog lock. Після commit зміни старий
worker не забирає та не скасовує нові jobs. Pending події старого hash скасовуються
після завантаження нової конфігурації й не перенаправляються на нову адресу.
Запит, розпочатий до зміни, може завершитися на попередньому одержувачі.

## Перевірки

PostgreSQL/HTTPS use case перевіряє If-Match, один active endpoint, inactive
replacement, atomic switch, missing-secret rollback, stale sender/test fencing,
readiness після runtime proof, delete та відсутність bootstrap resurrection.
Критичні HTTPS sender tests перевіряють TLS/bearer/no-redirect. Live Compose
сценарій виконує mixed apply, restart, читання збереженої конфігурації та реальну
HTTPS test delivery, потім відновлює початковий канал. React production build
перевірено; DOM clicks цим сценарієм не автоматизовані.
