# Historical observation detail R18

Дата: 2026-11-15 (Europe/Kiev). Source commit: eae041c; integration harness correction: 0fc8f63.
Project: nmo-observation-r18-final, private prefix 172.19, PostgreSQL та два mTLS Agent.
Host ports не публікуються. Image digest у image.txt.

Live scenario перевіряє GET observations/{id} на фактичному результаті,
рівність DTO з історією та CLI get observations ID після перезапусків Controller.
Збережені попередні критичні сценарії: auth/scopes/CSRF, desired apply/revision ACK,
Agent reconnect, diagnostic start/cancel, alert lifecycle/acknowledgement,
webhook outbox retry та dashboard. Публічний JSON містить лише результати перевірок,
без credentials, DSN, приватних ключів або cookie.

Окремий disposable PostgreSQL/HTTPS smoke перевірив historical revision,
execution failure vs probe failure, false/zero values, diagnostic task/session
provenance, помилкові запити та відсутній resources:read. Go build/vet,
критичні HTTP tests і React production build пройшли. DOM interaction не тестувався.

Для повторення використовуйте новий власний project без users: lab.py prepare,
lab.py up, потім lab_sessions.py --output LOCAL_PRIVATE_FILE. Завершуйте через
lab.py down; data volumes зберігаються. Відкриті модулі повного v1alpha1 описані
в IMPLEMENTATION_STATUS.md. Цей етап не означає завершення всього продукту.
