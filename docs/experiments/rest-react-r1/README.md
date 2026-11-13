# REST/React R1: реальна інтеграція

Перевірено робоче дерево після 5fc0b42 із першим REST/React етапом.
process-smoke.log: Controller, два mTLS Agent, окрема тимчасова PostgreSQL,
trusted HTTPS GET Assessment, 401 без token, production index/assets.
Також виконано replay/dedup, recovery, TTL та реальні PostgreSQL tests.

compose.json: окремий Docker-прогін із новим pinned Node/Go image. Python
client у власній office-net namespace перевірив TLSv1.3 із public CA та
server_hostname=localhost, API, 401 і два production assets без host ports.
Token передавався лише через stdin operator tool і не експортувався.
Assessment окремо валідовано через jsonschema за фактичним OpenAPI.
verification.json фіксує image та межі перевірки.

React/TS production build пройшов. Автоматизації кліків браузера не було;
віддача production assets не є доказом повного UI E2E. R1 не реалізує
повні user sessions/PAT CRUD/RBAC/configuration CRUD. Це working read-only
зріз, не scaffold і не повний продукт. Cookie API не імітується.

Відтворення: scripts/distributed_smoke.py --http-ui та scripts/lab_web.py
на підготовленому стенді. API/UI використовують той самий Controller/PG;
frontend не обчислює Assessment. Runtime-секрети й приватна PKI не входять
до цих файлів. Новий обсяг: docs/specs/thesis-scope-2026-11-13.md.
