# Estimated-byte budget — натурне приймання

Owned project nmo-bytes-20261119, справжні mTLS Agents, PostgreSQL і приватний
HTTPS API. Два Fixed profiles мають по два послідовні HTTP steps, maxChecks=3:

- maxEstimatedBytes=1: нуль physical tasks/Observations, estimatedBytes=0;
- точний ліміт 49187: один task, реальний success Observation, друга спроба
  не допускається; durable count=1, estimatedBytes=49187;
- обидва run inconclusive/no_admissible_action: невиконаний Fixed step не
  підмінено повним висновком;
- restart Controller зберіг public budget/task accounting; regular healthy.

Число 49187 походить із задокументованої [моделі](../../deployment/diagnostic-byte-budget.md),
не з вимірювання wire traffic. live.json містить public session/task evidence.

postgres.log — TestDiagnosticEstimatedBytesAtomicReservation на справжній БД:
лише одне з двох конкурентних reservation прийнято; session/task read models
мають 8192 estimated bytes; rejection не повертає резерв; terminal report його
зберігає. Перший запуск binary був перерваний restart Controller із live
сценарію (exit 137); після завершення сценарію окремий запуск успішний.
Go/race/vet і production image/React build пройшли. Початковий unit запуск
виявив помилку test fixture (confirmation вимкнено без stopWhenUnique);
fixture виправлено, збережений go.log — фінальний успішний запуск.
Без DSN інші PostgreSQL unit tests пропускаються. Нових browser clicks не було.

```sh
python3 scripts/lab.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_byte_budget.py --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python3 scripts/lab.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Перед up зібрати controller/target. Existing users/output не перезаписуються.
Для DB test зібрати CGO_ENABLED=0 go test -c ./internal/storage, надати test
binary mode 0755, скопіювати до /data Controller і запустити лише зазначений
TestDiagnosticEstimatedBytesAtomicReservation із приватним
NETMONORC_TEST_DSN_FILE=/run/netmonorc/database.dsn після live сценарію.
DSN/credentials не публікуються. Стенд зупинено, PKI/.local/volumes збережено.
