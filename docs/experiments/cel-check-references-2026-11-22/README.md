# Static CEL check references: SC-18

Підстава: DSL SC-18, V012 та §18; записка §2.1–2.4 про декларативні перевірки,
узгодженість конфігурації та відокремлення технічної помилки від стану цілі.
[Контракт і межі](../../deployment/cel-check-references.md).

Owned project `nmo-cel-refs2-20261122`: production Controller/Agent/CLI,
справжня PostgreSQL, HTTPS API, mTLS і реальні probes із двох мережевих зон.
Початковий Monitor research-web уже мав healthy. Через API отримано його
source/ETag, додано правило з checks["htttp"], потім виконано такі дії:

- config/validate повернув valid=false із повідомленням rule http-down
  references unknown check "htttp";
- config/plan, config/apply та PUT Monitor повернули 422;
- повторний GET мав початкові ETag та desiredRevision: некоректна конфігурація
  не замінила чинну;
- production offline CLI також відхилив помилковий Monitor; правильний
  checks["http"] пройшов. Для автономного одиночного CLI-прикладу diagnostics
  вимкнено, оскільки його зовнішній DiagnosticProfile не передається;
- production CLI прийняв публічний тридокументний YAML із CheckProfile,
  AssessmentPolicy та Monitor, але відхилив web__htttp замість web__http;
- PUT із виправленим правилом прийнятий; нова точна revision досягла healthy
  на реальних Agent observations. Cookie-сесію завершено.

`live.json` містить валідацію, HTTP-коди, CLI validation summaries і новий
Assessment. `runtime.json` фіксує base commit та фактичний образ. Паролі,
cookies, PKI та DSN до доказів не включено.

`go.log` / `vet.log`: повні go test ./... та go vet ./... із exit=0.
Unit cases охоплюють index/dot/has/membership, вкладені AST/macro, недосяжну
гілку, локальне затінення checks, динамічний ключ, policy binding без явних
checks, явний policy list та кінцеві IDs після CheckProfile expansion.
DSN-залежні unit tests за звичайного запуску можуть пропускатися; наведені
REST assertions виконані на живій PostgreSQL. `build.log`: production Docker
build Controller/Agent/CLI та включеної React-панелі.

Відтворення (потрібні зібрані образи, Docker та вільний приватний prefix):

```sh
python scripts/lab_cel_references.py prepare --project nmo-cel-refs-new --prefix 172.27
python scripts/lab_cel_references.py verify --project nmo-cel-refs-new --prefix 172.27 --output .local/cel-refs-new/live.json
python scripts/lab_cel_references.py down --project nmo-cel-refs-new --prefix 172.27
```

Перший helper trial виявив відсутність зовнішнього DiagnosticProfile у
одиночній offline CLI fixture. Fixture виправлено; фінальний сценарій пройшов
на окремому свіжому project. Обидва стенди зупинено; `.local`, PKI та volumes
збережено. Нових browser clicks немає. Це перевірка статичних імен checks;
computed keys, фактична наявність metrics/values та freshness лишаються
предметом runtime CEL. Повної готовності всього ТЗ не заявлено.
