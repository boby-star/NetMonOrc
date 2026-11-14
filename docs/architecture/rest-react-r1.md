# REST + React: перший наскрізний етап R1

Історичний опис R1. Парольні сесії та зміна власного пароля вже додані
в R2: [інструкція](../deployment/browser-sessions.md). Межі нижче стосуються R1.

Реалізовано GET /api/v1/monitors/{name}/assessment за наявним OpenAPI.
Controller читає збережений Assessment PostgreSQL, перетворює DTO, повертає
monitorId/state/coverageState/zoneStates/reasonCodes/evidenceIds/evaluatedAt/revision.
Відсутній або старший 5s знімок повертає 503, а не застарілий healthy. Одночасно допускається одне читання API,
щоб не займати весь PG pool: конкурентний запит отримує 429/Retry-After.
Це межа freshness HTTP snapshot, не заміна TTL probes.

Listener запускається лише з -http-listen, завжди HTTPS TLS≥1.3. Machine
mTLS stream лишається окремим. Bootstrap opaque bearer token з приватного
regular file без symlink відкриває лише читання цього endpoint. Це не повна
реалізація user sessions/PAT CRUD/RBAC; /auth/login і mutating endpoints
не реалізовані. Token не передається аргументом; -http-token-file — шлях.
Оператор повинен створити випадковий URL-safe token щонайменше 32 символи
і встановити mode 0600. Не використовуйте Agent enrollment token як reader.

Приклад додаткових flags до наявного Controller з cert/key/PG/Monitor:

```text
-http-listen=127.0.0.1:8443
-http-token-file=.local/viewer.token
-http-ui-dir=web/dist
```

За відсутності http-listen listener вимкнений. Без ui-dir можна використовувати
REST окремо. HTTPS certificate/CA має бути довірений браузером; не вимикайте
TLS verification. UI та API same-origin, CORS не відкривається. Credentials
через query string, localStorage чи cookies не використовуються цим етапом.

React + TypeScript знаходяться у web/, точні версії й integrity — package-lock.
Після `npm ci --ignore-scripts` та `npm run build` каталог web/dist передається
Controller. Використовується Node 24. Панель приймає ім'я Monitor і token
читання, зберігає token тільки в пам'яті вкладки, запитує сервер кожні 5s з timeout 8s і скасуванням запитів.
Вона показує серверні зональні стани/H/Q/coverage/revision/evidence, не дублює
математичний алгоритм у JS. Error очищує останню оцінку; stale snapshot
позначається. Кольори відповідають ТЗ: #2E8B57 / #FDF5E6.

Поточна UI — перегляд одного Monitor. CRUD, Agent registry, diagnostics й
alerts в React ще не готові. Повний обсяг: ../specs/thesis-scope-2026-11-13.md.
Bootstrap reader не слід видавати за повну production auth-підсистему.

Перевірка:

```bash
npm --prefix web ci --ignore-scripts
npm --prefix web run build
go build -o .local/netmonorc-controller ./cmd/netmonorc-controller
go build -o .local/netmonorc-agent ./cmd/netmonorc-agent
python3 scripts/distributed_smoke.py --http-ui
```

Скрипт використовує реальну PG, Controller, два mTLS Agent та довірений
HTTPS CA: перевіряє live Assessment DTO, 401 без token, index/production assets.
Browser interaction цим smoke не перевіряється; успішна збірка не замінює E2E.

## Compose

React збирається Node 24 builder stage з image digest та npm lockfile,
готові статичні файли входять до Controller image. npm працює з TLS verification;
корпоративний build CA передається тим самим BuildKit secret build_ca, що Go.
Вихідний code/node_modules не потрапляють до scratch runtime.

Для наявного власного стенда: down → build → prepare → up. Prepare додає
приватний `.local/lab/<project>/viewer-token` і копію лише до Controller volume;
паролі/PKI/дані не перезаписуються. За замовчуванням HTTP-порт теж не публікується.
Локальний браузер можна підключити явно:

```bash
docker compose -p netmonorc-lab -f compose.yaml -f deployment/web-panel.compose.yaml up -d
```

Відкрийте https://localhost:8443, довіряючи public CA вашого research stand.
Прочитайте токен із власного приватного viewer-token файла та вставте в панель;
не передавайте його через URL або незахищені CLI-аргументи. Для іншого project
вкажіть відповідні -p/NMO_NET_PREFIX. Порт публікується лише на loopback хоста
Docker daemon; remote Cloud daemon може не збігатися з хостом браузера.
