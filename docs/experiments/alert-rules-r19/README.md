# Managed AlertRule integration R19

Дата: 2026-11-15 (Europe/Kiev). Реалізація: f8608f4; сценарій: f404208.
Project: nmo-alert-rules-r19-verified, private prefix 172.19, PostgreSQL та два
mTLS Agent. Host ports не публікуються. Image digest — image.txt.

Сценарій змінює опис збереженого AlertRule через CLI apply --wait,
перевіряє reason=alert_rule_evaluated, перезапускає Controller і підтверджує
збереження зміни. Далі додає друге правило, очікує його реального оцінювання,
видаляє неактивне правило через ETag/CSRF API та відновлює початкове основне
правило. Після restart тимчасовий приватний CLI context створюється повторно
через token-stdin; його секрети не входять до evidence.

Попередні сценарії збережені: auth/scopes/CSRF, desired apply/revision ACK,
Agent reconnect, diagnostic start/cancel, alert lifecycle/acknowledgement,
webhook outbox retry, dashboard та observation detail.

Окремий disposable PostgreSQL/HTTPS smoke перевірив reference validation,
If-Match 428/412, fencing попереднього/deleted evaluator, firing/resolved,
409 на видалення активного правила та завершення apply лише після оцінювання.
Go build/vet і React production build перевірені. DOM clicks не автоматизовані.

Повторення: новий project без users; lab.py prepare → lab.py up →
lab_sessions.py --output LOCAL_PRIVATE_FILE. Після перевірки lab.py down
зберігає named volumes. Відкритий повний scope — IMPLEMENTATION_STATUS.md.
