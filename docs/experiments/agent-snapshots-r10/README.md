# Agent snapshots R10

Робоче дерево після dd85cbb. Приватний project nmo-agent-view-r10,
prefix 172.20, production image (image.txt). REST/CLI читають actual mTLS
sessions: heartbeat/cert expiry/current applied revision та load gauge.
Перевірені scope agents:read, zone filter, відсутність credentials,
невідомий ID. Реальний branch process зупинений: snapshot показує offline;
після start, нового session generation та heartbeat знову online.
Regular monitoring відновлено. Попередні critical CLI/apply/diagnostics/
users/PAT/revision scenarios проходять. Credentials не експортуються.
React production build пройшов; browser DOM clicks не автоматизовані.

Для відтворення потрібен новий власний disposable project без users,
scripts/lab.py prepare/up та scripts/lab_sessions.py з project/prefix.
Enrollment/managed Agent metadata ще не позначаються реалізованими.
