# Browser sessions: критичні юз-кейси

Перевірено робоче дерево після 2f8bcfc з Argon2id та durable sessions.
postgres.log: real PG + HTTPS, bootstrap idempotency, Argon2id, cookie
flags, CSRF, role scopes, expiry, logout revocation й lockout після
нового Service. live.json: окремий Docker project nmo-auth-r2 (172.28),
bootstrap admin через stdin, real HTTPS login, cookie Assessment read,
Controller restart і той самий session/CSRF, заборона logout без CSRF,
успішний logout і відмова revoked cookie. Усі credentials ephemeral
і не включені в файли. Browser DOM interactions не автоматизовані.

Це основа сесій, не завершені PAT/users CRUD/RBAC всіх майбутніх операцій.
