# Shared AssessmentPolicy integration R23

Дата: 2026-11-15 (Asia/Jerusalem). Source commit: d63112d.
Project: nmo-assessment-policy-r23, prefix 172.19, PostgreSQL, HTTPS Controller
і два mTLS Agent. Host ports не публікуються. Image digest у image.txt.

CLI mixed apply створив AssessmentPolicy і Monitor.assessmentRef. Після
actual revision ACK та свіжих probes Controller повернув healthy. Policy-only
update додав CEL rule з degraded: operation отримала implicit Monitor target,
дочекалася required_agents_applied, live assessment став degraded, власний
manifest/resourceVersion Monitor не змінився. Після restart policy/ref і стан
завантажилися з PostgreSQL. Відновлення policy дало healthy; наприкінці original
inline Monitor відновлено, unreferenced policy видалено, healthy та live Agent
proofs підтверджено. Попередні critical workflows R1–R22 також перевірені.

Pure tests: required failure при H=0.95 навіть за healthy CEL; missing proof
не стає healthy; oneOf/ref/defaults; canonical revision/Agent VerifyPlan;
фактичні Target descriptors/labels у CEL та revision change за зміни labels.
PG/HTTPS: unordered mixed apply, JSONB normalization, delete/If-Match guards,
policy-only implicit revisions, source version preservation, old healthy fencing.
Go build/vet і React production build пройшли. DOM clicks не автоматизовані.
Математичний профіль лишається all-required/all/weighted/unknown fallback;
додаткові modes не оголошуються реалізованими.

JSON export не містить credentials, cookie, DSN чи private keys. Повторення
потребує нового owned project без users: lab.py prepare → up → lab_sessions.py.
Після down named volumes зберігаються. Повний scope — IMPLEMENTATION_STATUS.md.
