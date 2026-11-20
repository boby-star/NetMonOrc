# Declarative Component — натурне приймання

Свіжий owned project nmo-components-r2-20261120, приватний HTTPS, mTLS Agents,
справжні PostgreSQL і Blackbox Exporter 0.27.0. Окремі exporter services живуть
у мережевих namespace office/server; вони не встановлюються через Component.
Production CLI component-config підготував content-addressed modules перед up.

live.json підтверджує:

- шість regular Agent/check pairs отримали completed/success через binding;
- інший DiagnosticProfile дав completed/responding, один physical task і доказ;
- stop обох зовнішніх exporter дав шість execution_failed/unknown/
  runner_unavailable, Assessment unknown, без fallback до доступного binary;
- disabled binding дав unknown; restart зберіг disabled revision;
- re-enable відновив healthy;
- delete Component повернув явно задокументований legacy local runner,
  початкову effective revision та healthy;
- Monitor source resourceVersion не змінювався від Component updates;
- managed/public endpoint/unknown moduleSet та overlapping selectors відхилено;
- logout успішний, credentials у звіті відсутні.

Go/race/vet пройшли; production build містить React editor. Нової browser
interaction перевірки не було. DB unit tests без DSN пропускаються; цей сценарій
виконував CRUD/reconciliation та probes на справжній PostgreSQL.

Перший project nmo-components-20261120 був зупинений після помилки test helper:
Docker cp файлового snapshot під час оновлення дав неповний JSON. Реальний
Assessment згодом був healthy з усіма шістьма позитивними probes. Helper тепер
використовує REST assessment; повне приймання виконано на новому r2 project.
Початковий compile виявив missing errors import, виправлено до final tests/build.
Збережені logs належать фінальній версії. runtime.json містить fingerprints.

```sh
docker compose build controller target
python3 scripts/lab_components.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_components.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_components.py verify --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python3 scripts/lab_components.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

При own build CA доступний deployment/build-ca.compose.yaml. Helper відмовляється
перезаписувати наявну робочу теку/users/output. Обидва owned projects зупинено;
.local, PKI та volumes збережено. [Контракт та межі](../../deployment/components.md):
немає proactive Component health protocol; unmatched Agents лишаються legacy;
це не доказ strict external-only або повної відповідності всіх полів DSL.
