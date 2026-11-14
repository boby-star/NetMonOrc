# Особисті токени та вхід CLI (R4)

Панель має парольний вхід, а CLI використовує особистий API-токен.
Агентські сертифікати, enrollment та bootstrap reader залишаються окремими.
Токен містить 256 випадкових бітів і префікс `nmo_pat_`. PostgreSQL зберігає
тільки SHA-256 secret. Сервер видає secret один раз у відповіді створення;
GET показує лише метадані. Назви токенів теж не повинні містити секретів.

## Створення та відкликання

Увійдіть до React-панелі з обліковим записом і відкрийте «Особисті API-токени».
Задайте назву, потрібні дозволи та строк дії. Початковий вибір — resources:read;
порожній строк означає токен без expiry. Дозволи не можуть перевищувати
актуальну роль власника. Після видачі збережіть secret у захищеному місці
та натисніть «Приховати секрет». Секрет лишається лише в пам'яті компонента,
без localStorage/sessionStorage. Відкликання відразу забороняє нові запити
токеном; уже авторизовані запити можуть завершити свою поточну операцію.

| REST | Поведінка |
|---|---|
| POST `/api/v1/auth/tokens` | Лише browser cookie + CSRF; 201 metadata + одноразовий token |
| GET `/api/v1/auth/tokens` | Метадані лише власних токенів, limit/cursor |
| DELETE `/api/v1/auth/tokens/{id}` | Власний token, 204; повторне відкликання 204 |
| GET `/api/v1/auth/me` | Роль, ефективні scopes, expiry; для PAT без expiry — null |

POST приймає name, scopes, expiresAt; невідомі поля відхиляються. Назва:
1..128 байтів UTF-8 без крайніх пробілів та control characters. Пропущені
або null scopes дають resources:read; явний [] не дає доступу до ресурсів.
Відомі scopes перелічені в SessionView/OpenAPI, повтори заборонені.
Явний expiresAt має бути в майбутньому; null/пропуск — без expiry.

Список сортується ID DESC, до 200 записів на сторінку (default 50), nextCursor
передається без аналізу клієнтом. Токени іншого власника не показуються;
спроба відкликати чужий/невідомий ID повертає 404. Власник автентифікується
сесією або власним PAT. PAT може керувати метаданими/відкликанням власних
токенів, але не випускати нові credentials. Для cookie mutations потрібен
X-CSRF-Token. Видача та перше відкликання записуються в identity audit,
без secret. Зміна пароля відкликає browser sessions; чинні PAT потрібно
відкликати окремо.

На кожний запит PAT перевіряються revoke/expiry/disabled та актуальна роль.
Ефективні дозволи — перетин записаних scopes і scopes цієї ролі. Невідомий
bearer ніколи не підміняється на browser cookie. Поточні endpoints Assessment
та observations потребують resources:read. Відсутній scope — 403, відхилений
credential — 401. Bootstrap reader має доступ лише до Assessment.

## CLI

Збірка: `go build -o .local/netmonorc ./cmd/netmonorc`.

```sh
.local/netmonorc login --server https://localhost:8443 --ca-file /private/ca.crt
.local/netmonorc whoami
.local/netmonorc logout
```

Login запитує PAT через прихований terminal prompt. Для автоматизації є
`--token-stdin`: передавайте secret захищеним stdin, не літералом командного
рядка і не environment. Перед записом контексту CLI перевіряє токен через
реальний `/api/v1/auth/me`. Сервер — лише HTTPS origin без URL credentials,
path або query. Redirect заборонений, TLS перевіряється завжди (мінімум 1.3).
CA доповнює системний trust store; прапорця insecure немає.

Контекст за замовчуванням: каталог користувацьких налаштувань ОС,
`netmonorc/context.json`. Для окремої інсталяції можна явно передати
`--context-file /private/context/context.json` усім трьом командам.
Файл атомарно записується як 0600 у приватний каталог 0700; symlink і
публічно читаний credential-файл відхиляються. Whoami повертає JSON без
secret. Logout прибирає лише локальний файл; для відкликання на сервері
скористайтеся панеллю. Перезапуск процесу CLI знову читає той самий контекст.

Це реалізація login/whoami/local logout. Загальні named contexts, Cobra,
усі read/CRUD команди та режими output ще не завершені; існуючі validate,
probe, assess залишаються локальними дослідницькими операціями.

## Перевірка

`python3 scripts/identity_smoke.py` запускає один критичний тест із власною
PostgreSQL та HTTPS: one-time secret/hash at rest, scopes, downgrade,
expiry/revoke, ownership, pagination, cookie/CSRF та попередні сценарії
password/history. Окремий `go test ./internal/apiclient` перевіряє захищений
контекст, trust TLS, відмову redirect і local logout. React production build
перевіряється TypeScript/Vite. Browser DOM clicks не автоматизовані.
