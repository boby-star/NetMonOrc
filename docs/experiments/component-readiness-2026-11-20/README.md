# Component readiness — натурне приймання

Owned project nmo-readiness-20261120, справжні Agent/mTLS, Blackbox Exporter,
PostgreSQL та приватний HTTPS API. Оновлений scripts/lab_components.py готує
модулі production CLI та виконує external Component сценарій із readiness.

live.json підтверджує:

- свіжі ready звіти для точних revision/ref на обох Agents у REST agents;
- шість positive regular probes та один completed physical diagnostic task;
- stop зовнішніх exporter → свіжі unavailable звіти й regular unknown;
- explicit manual start під час unavailable → inconclusive/no_admissible_action,
  durable additionalChecks=0 та порожній tasks endpoint (blockedDiagnostic,
  blockedTasks), без перетворення technical failure на Target DOWN;
- start exporter → нові ready timestamps і healthy;
- disabled readiness на обох Agents, restart зі сталою disabled revision;
- re-enable, fresh ready та healthy; delete binding відновлює legacy local mode;
- Component health не переписує Monitor source resourceVersion; logout пройшов.

Unit tests окремо підтверджують session/revision/ref/hash/timestamp fencing,
reject duplicate/forged/stale/future reports, unknown при відсутності/протуханні,
ігнорування withdrawn applied revision, shared admission deny до physical task,
endpoint dedup, disabled без мережі та health тільки /-/healthy з bounded body
і без metadata redirect. Generated protobuf серіалізацію перевірено реальним
Agent stream; ручного редагування generated Go не було.

Go/race/vet пройшли. Без DSN частина storage unit tests пропускається; наведені
run/task/reservation assertions використовували справжню PostgreSQL. React
увійшов у production build; нових browser clicks не було. runtime.json містить
image ID та fingerprints прийнятої реалізації. Health сам собою не доводить
наявність кожного generated module або здоров'я цілі.

```sh
docker compose build controller target
python3 scripts/lab_components.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_components.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_components.py verify --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python3 scripts/lab_components.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Helper не перезаписує existing work/users/output. Стенд зупинено;
.local, PKI та volumes збережено. [Протокол та межі](../../architecture/component-readiness.md).
