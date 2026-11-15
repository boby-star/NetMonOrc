# Scoped dashboard overview R17

Дата 2026-11-15 (Europe/Kiev), реалізація 6a57389. Production image
в image.txt. Project nmo-dashboard-resume-r17, private prefix 172.19, PostgreSQL і
два mTLS Agent; host ports не публікуються.

CLI/API показують healthy=1, sufficient=1, online=2, firing=0 та реальні recent
diagnostic sessions після відновлення stand. Resource-only PAT не отримує
agent/alert/diagnostic поля. Попередні critical user workflows збережені.

Real PG/HTTPS перевіряє fresh active + unknown unselected, stale healthy,
desired revision rollout і незалежність Agent online від resource state.
Scopes і query validation перевірені. Детальний Assessment API використовує
той самий projection/freshness helper. Go build/vet, критичні тести й React
build пройшли. DOM clicks не автоматизовані; credentials/keys не експортуються.
Для повторення потрібен новий власний project без users; data volumes
після down зберігаються. Відкритий scope у IMPLEMENTATION_STATUS.md.
