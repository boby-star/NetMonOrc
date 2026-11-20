# ExecutionFallback — натурне приймання

Два власні стенди з PostgreSQL, mTLS Agents branch/dc, приватним HTTPS API
та fixture, без публічних target. Фіксований primary=branch, spare=dc, один
HTTP Blackbox check з timeout=2s/interval=10s, maxAttempts=2. Diagnostics disabled:
fallback дозволений власним блоком check. Для same-zone обидва Agent в office;
для different-zone spare у server. Labels задаються Controller.

same-zone.json та different-zone.json містять:

- HTTP 503 → down, жодного fallback run; після відновлення fixture healthy;
- disabled Component лише primary → execution_failed/unknown, original
  Observation збережено, один окремий physical task на spare; той дав success;
- actual Agent/zone та root/task links збігаються у REST історії;
- same-zone повертає healthy і включає fallback proof в Assessment;
  different-zone лишає office unknown і не включає цей proof до coverage;
- після видалення тимчасового Component повертається початковий revision/healthy;
- stop primary → agent_unreachable, один task, originObservationId порожній:
  непідтверджене фізичне виконання не перетворене на вигаданий Observation;
- після start primary healthy, незмінні Monitor ETag/effective revision,
  жоден root не перевищує maxAttempts, logout успішний.

postgres.log — окремий запуск TestFallbackDurableOwnershipAndNoBudgetRefund
на справжній БД після live trial. Idle spare не має regular permission, але
може повернути точно зарезервований fallback. Forged check відхиляється,
повторний payload idempotent, maxAttempts=1 не обходиться, root replay не
повертає budget, regular і fallback history розділені, physical provenance
збережено. Rejection чужої revision/старої generation не фіксується; поточна
assignment приймається; наступне успішне apply прибирає stale rejection. Expiry persist-иться, повторне відкриття не відновлює
pending і не витрачає task. Цей тест не заявляє live config_rejected trial.

Go/race/vet пройшли. Звичайний go test пропускає DSN-залежні tests; лише вказаний
вище DB test виконувався зі справжнім приватним DSN. Agent test перевіряє точний
check, origin/zone/correlation, budget, заборону звичайної diagnostics і receipt
replay. Config test перевіряє closed schema, типи/causes/duplicates та canonical
hash/default compatibility. Monitor tests захищають zone/replica coverage.
Production build включає React; нових browser clicks не було. CLI перевірив
повний YAML приклад (example-validation.json).

runtime.json відрізняє image натурних trials від фінального: після trials додано
спільний read limit для API detail, durable expiry та індекси, а також
серіалізацію rejection з generation і очищення rejection після успішного apply. final-smoke.json
підтверджує повторний запуск фінального image на обох наявних стендах і healthy.
Повні fault trials на фінальному image вдруге не запускались. Build/test logs
нормалізовано лише trailing spaces. SHA256SUMS фіксує вміст усіх цих артефактів.

Перший запуск виявив старий bootstrap compiler, що відхиляв placement; його
замінено спільним registry перед успішними trials. Під час роботи закінчилось
місце у build caches: очищено відтворюваний Go cache і невикористаний Docker
builder cache. PKI, .local, named volumes та дані стендів не видалялись.

Відтворення (кожна стратегія потребує нового project/незайнятого prefix):

```sh
python scripts/lab_fallback.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --strategy same-zone
python scripts/lab_fallback.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python scripts/lab_fallback.py verify --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python scripts/lab_fallback.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Для іншої зони prepare --strategy different-zone. Helper не перезаписує users
або output. Для DB test зібрати CGO_ENABLED=0 go test -c ./internal/storage,
надати binary mode 0755 і виконати лише зазначений test у своєму Controller
з NETMONORC_TEST_DSN_FILE=/run/netmonorc/database.dsn; DSN не друкувати.
Стенди зупинено, volumes/.local/PKI збережено.

Це один Docker host із логічними зонами; немає HA/scale benchmark, окремих live
fault trials config rejection/всіх local errors чи доведення exactly-once.
[Контракт](../../architecture/execution-fallback.md) визначає точні межі.
