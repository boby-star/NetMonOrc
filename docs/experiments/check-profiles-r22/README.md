# Typed CheckProfile integration R22

Дата: 2026-11-15 (Asia/Jerusalem). Source commit: 52d9d40.
Project: nmo-check-profiles-r22-final; private prefix 172.19.
PostgreSQL, HTTPS Controller та два mTLS Agent; host ports не публікуються.

CLI mixed apply створив CheckProfile та Monitor.useProfiles. HTTP/TCP/DNS
checks отримали base__ namespace; Agents прийняли IR checksum і повернули
healthy assessment. CheckProfile-only default update expectedStatus=503
створив implicit Monitor target, дочекався heartbeat-backed ACK та змінив
оцінку на down. Source manifest/resourceVersion Monitor залишився незмінним.
Після restart binding і default завантажилися з PostgreSQL. Відновлення
профілю повернуло healthy; потім original inline Monitor відновлено й
unreferenced CheckProfile видалено. Фінальні Agent proofs підтверджені.

Збережено попередні critical use cases R1–R21. Export JSON містить лише
результати перевірок, без credentials/cookie/DSN/private keys.

Pure tests перевіряють typed bindings/defaults, partial/outside-config template
rejection, missing/wrong/unknown parameters, collisions/64-character ID limit,
canonical hash, Agent VerifyPlan і YAML ambiguity/cycle/number preservation.
Real PostgreSQL/HTTPS перевіряє unordered mixed apply, JSONB float normalization
(включно з 1e100), reference delete guard, source version preservation та
profile-only IR changes. Go build/vet і React production build пройшли.
DOM clicks не автоматизовані. Повний відкритий scope — IMPLEMENTATION_STATUS.md.

Для повторення створіть новий owned project без users: lab.py prepare → up →
lab_sessions.py --output LOCAL_PRIVATE_FILE. Після down named volumes збережені.


Після live прогону додано input expansion guards (017fa0f). Вони перевірені
цільовим unit test переповнення та повторним real PG/HTTPS use case; Go build/vet
пройшли. image.txt — точний live-tested image для 52d9d40; guard-build-image.txt —
пізніша успішна збірка з guards. Повторний повний Compose для цього додаткового
input guard не виконувався. Обидва image digests збережені окремо.
