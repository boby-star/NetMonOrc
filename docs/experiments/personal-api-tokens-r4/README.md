# PAT, React та CLI: критичні сценарії R4

Перевірено робоче дерево після b803f1f із PAT та CLI identity.
postgres.log: один real PG + HTTPS тест, включно з issuance/CSRF, scopes,
current-role intersection, disabled owner, expiry/revoke, ownership,
metadata pagination, hash at rest та попередніми password/history сценаріями.
client.log: trusted TLS, заборона redirect/plain HTTP, private atomic
context, відмова symlink/public permissions і local logout.

live.json: новий приватний Compose project nmo-pat-r4, prefix 172.26,
production image (image.txt), Controller/PostgreSQL/два mTLS Agents/Blackbox
та зібрана React-панель. Через реальний HTTPS видано PAT без expiry,
прочитано Assessment/history, виконано CLI login/whoami/local logout через
захищений stdin. Після restart Controller PAT лишився чинним. Відкликання
через browser session зробило PAT непридатним. Попередні перевірки пароля,
сесій, CSRF та history також проходять. Credentials ephemeral, не збережені.

Команди:

```sh
python3 scripts/identity_smoke.py
go test ./internal/apiclient
python3 scripts/lab.py prepare --project nmo-pat-r4 --prefix 172.26
python3 scripts/lab.py up --project nmo-pat-r4 --prefix 172.26
python3 scripts/lab_sessions.py --project nmo-pat-r4 --prefix 172.26 --output .local/pat-live.json
```

Lab sessions потребує нового власного disposable project без users;
повторно використовуйте нове ім'я, не видаляйте чинні identities.
React пройшов TypeScript/Vite production build. Browser DOM clicks не
автоматизовані. Ці перевірки не означають завершення всього v1alpha1.
