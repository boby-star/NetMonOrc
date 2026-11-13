# ADR 0005: перший REST/React зріз у Controller

Статус: прийнято за прямим дорученням користувача розширити прототип.
Повний обсяг визначено docs/specs/thesis-scope-2026-11-13.md.

REST і static React assets обслуговує той самий Go Controller на окремому
HTTPS listener. Agent mTLS stream лишається окремим. Бізнес-оцінювання
не переходить до HTTP handlers або браузера: adapter читає PostgreSQL
Assessment і формує DTO на вже визначеному OpenAPI route.

Для R1 обрано тільки GET Assessment із bootstrap reader token з приватного
файла. Доступ до інших routes не надається, mutation не імітується. Token
не є enrollment credential і не передається незахищеним CLI-аргументом.
Повні users/sessions/PAT/scopes/CSRF/audit — наступний R2, не позначений
готовим. Bootstrap допускається лише як явно обмежений operator mode.

Перший UI показує стан/coverage/H/Q/revision/evidence одного Monitor та
обробляє недоступний/застарілий snapshot. Кожні 5s виконується typed fetch;
backend обмежує одночасне читання до одного, повертаючи 429 конкурентному.
Assets зібрані з npm lockfile у pinned Node builder, scratch runtime не
містить Node/npm. Default Compose не публікує HTTPS на хост; override
публікує лише loopback. Обидва підходи не додають нового мікросервісу.

Верифікація: API/secret/authorization/freshness tests, TypeScript production
build, trusted HTTPS + real PG/mTLS Agents, OpenAPI response validation та
контейнерна віддача React assets. Автоматизовані browser interactions не
перевірені й не позначаються виконаними.
