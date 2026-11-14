# Diagnostic session history R12

Робоче дерево після 215e482, 2026-11-14. Фінальний production image записаний
в image.txt. Compose project nmo-diagnostic-read-r12, private prefix 172.19,
PostgreSQL і два справжні mTLS Agent. Стенд не публікує host ports.

scripts/lab_sessions.py читає реальні автоматичні runs через CLI get diagnostics,
деталі однієї сесії та get --tasks. Перевіряються profileRef, monitor,
кількість фізичних reservation/tasks, attemptNo та посилання на отримані
спостереження. PAT без diagnostics:read не може читати історію. Сценарій
також зберігає попередні auth/users/history/config/apply/profile/reconnect
перевірки. Дані в live.json — тільки санітизовані результати.

scripts/identity_smoke.py запускає disposable localhost PostgreSQL і критичний
HTTPS use case: equal-time keyset, прив’язка cursors до filters/session,
legacy static/restart без report/profileRef, reserved budget з неповними
Observation, без check_spec/body у DTO, invalid filters, missing ID, scope
і відсутність мутації через read endpoint. Це перевірка read model;
математичний planner не змінювався.

Go build/vet, тести зачеплених пакетів, TypeScript/Vite build пройшли.
Автоматизовані browser DOM interactions не виконувалися. Для відтворення
потрібен власний новий project без users: lab.py prepare/up, потім
lab_sessions.py з відповідними project/prefix. Після down data volumes
залишаються. Credentials/DSN/keys не входять до цього артефакту.

POST diagnostics start/cancel цим етапом не реалізовані. Один active Monitor
і fixed roster залишаються поточними runtime обмеженнями.
