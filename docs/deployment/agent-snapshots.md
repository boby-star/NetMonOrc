# Огляд агентів у REST, React і CLI (R10)

GET `/api/v1/agents` та GET `/api/v1/agents/{id}` читають fixed roster і
поточні mTLS-сесії Controller. Потрібний agents:read із browser session/PAT;
bootstrap reader або PAT лише з resources:read не має такого доступу.

Рядок містить фактичний ID із roster, zone, стан підключення, оголошені
capabilities, підтверджену ревізію, останній heartbeat і строк peer certificate.
MaxConcurrentProbes та activeProbes беруться з валідованого heartbeat як
capacity/current gauge. Поки такої telemetry немає, API повертає null, а
панель — «—». Ключі, tokens, CSR чи credential hashes не повертаються.

online означає applied поточної сесії, heartbeat не старший 15s та чинний peer
certificate. Підключена сесія без потрібних свіжих підтверджень має degraded.
Відсутня сесія має offline; це відсутність підключення, не доказ смерті процесу
або down контрольованого сервісу. Дані попередньої disconnected сесії не
видаються за поточні. Capabilities — оголошення агента, не runner health.

Roster залишається статичним, з identities дослідницького профілю, а не
повною enrollment Registry. Labels порожні, failureDomain не вигадується.
Enrollment/revoke/patch metadata та labelSelector ще не реалізовані. Методи
snapshot read-only, версії managed metadata/ETag не підміняються telemetry.

Список має limit 1..200 (default 50), ID ASC/cursor, zone/state. Наступний
cursor передається без розбору. Невідомий Agent ID повертає 404. Read snapshot
копіюється під mutex; heartbeat та generation fencing зберігають наявні правила.

React показує розділ «Агенти» для agents:read. Оновлення ручне з часом знімка,
фільтрами та сторінками; відсутність відповіді очищає таблицю і показує error,
а не старий online. У рядку розділені connection, heartbeat, applied revision
і load. Enrollment або керування cert цим екраном не імітуються.

```sh
netmonorc get agents
netmonorc get agents branch
netmonorc get --state online --limit 20 agents
```

CLI використовує той самий захищений PAT context і TLS transport. У Compose
перевірені API/CLI snapshots, read scope, zone filter, невідомий ID, реальне
відключення branch та повернення online після fenced reconnect/heartbeat.
Попередні apply/diagnostics та regular assessment проходять у тому самому
сценарії. Browser DOM clicks не автоматизовані; production React build пройшов.
