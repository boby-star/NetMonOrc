# Trusted metadata та per-check placement — натурне приймання

Свіжий owned project nmo-placement-20261119; mTLS Agent, приватний HTTPS,
реальні Blackbox probes і PostgreSQL. [Контракт](../../deployment/check-placement.md).

live.json підтверджує:

- Config plan має три окремі checkId рядки;
- HTTP виконує лише branch/office; DNS/TCP — лише dc/server;
- healthy, complete coverage у двох зонах за цими трьома фізичними парами;
- PostgreSQL має три per-check selections та точні historical allowed_checks;
- restart Controller зберігає ревізію й вибір;
- selector, який не збігся, не запускає HTTP і залишає unknown;
- distinct failure domains дають healthy і шість Agent/check пар;
- shared або unknown домени дають unknown і нуль regular probes у новій ревізії;
- labels повертаються read API з Controller inventory;
- початкові домени/Monitor відновлено, отримано healthy, logout пройшов.

postgres-replay.log — додаткова Go-перевірка справжнього CommitBatch проти цієї
БД: після reconnect/нової ревізії відхилено старий результат чужого check,
а legitimate replay власного check прийнято. Тестові ідентичності унікальні.
Перший запуск самого test binary не стартував через mode 0700 і Docker copy
ownership. Після встановлення 0755 для створеного нами test binary перевірка
успішна; помилка запуску не була помилкою алгоритму чи доказом приймання.

Go packages, race для змінених runtime-пакетів та vet пройшли. Без окремого
PostgreSQL DSN більшість DB unit tests у go.log пропускаються; зазначений replay
тест і натурний сценарій виконані на реальній БД. Production image містить React
build. Нових browser clicks не було. runtime.json має image ID/base commit та
fingerprints коду до коміту. Logs нормалізовано лише видаленням кінцевих пробілів.

## Відтворення

```sh
docker compose build controller target
python3 scripts/lab_check_placement.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_check_placement.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_check_placement.py verify --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python3 scripts/lab_check_placement.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Helper використовує власний bounded Compose override/inventory. Для build із
власним CA є deployment/build-ca.compose.yaml. Existing users/config та output
не перезаписуються. Стенд зупинено, .local/PKI/named volumes збережено.

Для replay-test: зібрати `CGO_ENABLED=0 go test -c ./internal/storage`, надати
створеному test binary mode 0755, скопіювати до приватного /data Controller та
запустити лише TestPerCheckHistoricalAssignmentReplay із
NETMONORC_TEST_DSN_FILE=/run/netmonorc/database.dsn. Не публікувати DSN.

Це не HA/load acceptance чи доказ незалежності фізичних хостів; домени стенда
задані явно на одному Docker-host. Залишок інших YAML-функцій наведено в
[актуальному контракті](../../deployment/yaml-controls.md#фактичний-залишок-повної-таблиці-аудиту).
