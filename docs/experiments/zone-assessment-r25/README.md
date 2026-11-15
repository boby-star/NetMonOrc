# Explicit zone SLA and local evidence preservation (R25)

Дата: 2026-11-15 UTC. Runtime source: 60a5dcd. Project nmo-zones-r25,
private prefix 172.19, PostgreSQL, HTTPS Controller та два mTLS Agent.
Image digest: image.txt. Порти host не публікувалися.

Виконано вузький штатний workflow lab_sessions.py --zones-only. Fresh bootstrap,
login та PAT не обходять authentication. Monitor і AssessmentPolicy з any
створені mixed apply, diagnostics вимкнена на час контрольованих fault injections.
Обидва Agent ACK-нули ревізію й надали реальні Blackbox HTTP/TCP/DNS probes.

Office-only route prohibit до fixture VIP спричинив локальний down при healthy
server. Any повернув global healthy, зберіг office down і sufficient coverage
у REST. Policy-only all-required змінив revision, не source resourceVersion
Monitor; той самий fault дав global degraded. Відновлення маршруту повернуло
healthy. Після повернення any та stop branch TTL дав office unknown, global
healthy і degraded coverage. Restart Controller з offline branch зберіг policy
та SLA; cookie і PAT лишилися чинними до штатного revoke наприкінці сценарію.

Відновлення original all-required Monitor з offline branch дало global unknown
і degraded coverage. Reconnect branch повернув healthy обох зон; operation
completed лише після потрібних ACK. Unreferenced policy видалена, початкова
конфігурація/diagnostics відновлена. PAT і cookie відкликані через API,
локальний CLI context видалено logout. Експорт live.json не містить secrets.

Критичні Go tests: таблиця станів all-required/any, down+unknown→degraded,
required local failure не скасовується healthy CEL, any не приховує зональний
down/unknown, mode змінює revision, default/explicit all-required мають однаковий
hash. PG/HTTPS: any зберігається через JSONB, залежна ревізія й optimistic
locking, old healthy assessment fencing. Go vet internal/cmd та Docker build
усіх Go binary/React пройшли. Додатковий compiler-default assertion після image
build змінював лише tests; runtime code лишився тим самим.

Змін React у R25 немає: чинний API DTO і панель уже показують global state,
coverage та зональні результати. DOM automation не виконувалася. Не перевірялися
цим вузьким прогоном решта повного workflow R1–R24; вони не оголошуються повторно
пройденими. Same-zone replica voting, dynamic placement, multi-Monitor та
повний v1alpha1 лишаються відкритими.

Повторення на новому owned project:
lab.py prepare → lab.py up → lab_sessions.py --zones-only. Кожній команді
задайте однакові --project та --prefix; для результату — новий --output.
Після down named volumes зберігаються.
