# Shared Target integration R21

Дата: 2026-11-15 (Europe/Kiev). Source: 04d755f.
Project: nmo-targets-r21, private prefix 172.19, PostgreSQL і два mTLS Agent.
Host ports не публікуються. Image digest зафіксовано в image.txt.

Live scenario створив Target та пов'язав активний Monitor через unordered
mixed apply. Нову ревізію виконали реальні Agents і Controller отримав healthy.
Target-only criticality update створив implicit Monitor target, дочекався
heartbeat-backed ACK та нового assessment. Manifest resourceVersion Monitor
при зміні залежності залишився незмінним. Після restart Target/targetRef/hash
завантажилися з PostgreSQL; startup inline-файл не перезаписав desired.
Наприкінці inline Monitor відновлено, unreferenced Target видалено, live Agent
proofs підтверджено. Попередні critical use cases R1–R20 також пройшли.

Pure unit tests: closed schema/types/defaults, passive logical, false/zero attrs,
oneOf/missing ref, dependent hash та Agent VerifyPlan. Real PG/HTTPS: atomic
unordered apply, reference delete guard, implicit revisions, old healthy fencing.
Go build/vet і React production build пройшли. DOM clicks не автоматизовані.
JSON evidence не містить credentials/cookies/DSN/keys. Повторення потребує
нового owned project без users: lab.py prepare → up → lab_sessions.py.
Після down named volumes залишаються. Повний відкритий scope — IMPLEMENTATION_STATUS.md.
