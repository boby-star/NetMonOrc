# Керування користувачами: R5

Перевірено робоче дерево після 6d2c097 із Users API/React.
critical.log — один цільовий real PostgreSQL + HTTPS тест identity/users/PAT.
Users cases: create/login, duplicate/CSRF, admin role+system:admin scope,
заборона viewer, ETag 428/412, null/malformed input, disable/re-enable,
role change і revocation sessions/PAT, metadata pagination, legacy createdAt.
Дві паралельні транзакції блокують кожна свого адміністратора: рівно одна
проходить, друга отримує conflict, один active admin зберігається.

live.json — production образ (image.txt), приватний Compose project
nmo-users-r5, prefix 172.25. Реальний HTTPS: читання admin з ETag,
відмова блокування останнього admin, create viewer/login, viewer RBAC,
disable/reenable з ETag та відмова старим cookies/PAT після re-enable.
Попередні password/history/PAT/CLI/restart сценарії також проходять.
Credentials ephemeral та відсутні у файлах доказів.

```sh
python3 scripts/identity_smoke.py
python3 scripts/lab.py prepare --project nmo-users-r5 --prefix 172.25
python3 scripts/lab.py up --project nmo-users-r5 --prefix 172.25
python3 scripts/lab_sessions.py --project nmo-users-r5 --prefix 172.25 --output .local/users-live.json
```

Для повторного lab_sessions потрібний новий власний disposable project без
users; наявні identities не перезаписуються. Початковий запуск prepare був
заблокований повним диском Cloud. Після очищення невикористаного build cache
сценарій повторено успішно; код, images і data volumes збережені.
React production build та Go build/vet пройшли. Browser DOM clicks не
автоматизовані. Це завершений Users-зріз, не повна реалізація всього v1alpha1.
