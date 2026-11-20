# Parallel Fixed DAG — натурне приймання

Owned project nmo-fixed-parallel-20261120, справжні mTLS Agents, PostgreSQL,
приватний HTTPS API та fixture. Native HTTP probes left/right мають контрольовані
затримки 2s/1s; join не затримується. maxParallel=2, один diagnostic slot на Agent.
Вхідний порядок steps не є топологічним; scheduler його не використовує як execution order.

live.json містить п'ять окремих manual runs:

- overlap-1 / overlap-2: по три tasks, parents фізично перекриваються, right
  закінчується раніше left, join починається лише після обох. Журнал стабільно
  b-left,c-right,a-join. У двох запусках збіглися Step/action/Agent/attemptNo/
  hypothesesBefore; physical IDs/timestamps/durations навмисно не порівнюються
  як однакові. Run completed/responding, actual durable max_parallel=2;
- count: maxAdditionalChecks=1, один b-left task, inconclusive, child відсутній;
- bytes: точний estimate одного HTTP attempt, один task, рівно цей durable
  estimatedBytes, inconclusive; це оцінка за моделлю, не вимір wire traffic;
- cancelledWave: обидва slow tasks прийнято, operator cancel дає cancelled,
  usedChecks=2, обидва workers надали execution_failed/unknown. Join не запущено.

Regular Assessment після trial healthy, Monitor source version/effective
revision не змінені; logout успішний. CLI/React stepId прив'язаний до справжнього
physical task/Observation. Нових browser clicks не було; React входить у build.

postgres.log: TestDiagnosticParallelReservationIsAtomic на цій справжній БД
допускає рівно один із двох конкурентних Agent reservations при maxParallel=1.
Rejected task звільняє active slot, наступний reservation проходить, used count
стає 2 без refund, step_id збережено. Окремий byte test має maxParallel=2,
щоб byte cap перевірявся незалежно від active-slot cap; він також пройшов.
Test binary запускався після live trial, без паралельного Controller restart.

Go/race/vet пройшли. DB tests без DSN у go.log пропускаються; зазначені дві
перевірки виконано окремо на справжній PostgreSQL. Unit wave test примусово
міняє фізичний порядок завершення; перевіряє join, context snapshot, retry
count/byte quota, same-Agent retry, zero-cost skipped tail і cancellation/drain.
example-validation.json — перевірка нового YAML прикладу production CLI.
runtime.json містить image ID/fingerprints. Logs нормалізовано лише trailing spaces.

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_fixed_parallel.py --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python3 scripts/lab.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Helper не перезаписує users/output. Для DB test: зібрати CGO_ENABLED=0 go test -c
./internal/storage, надати власному binary mode 0755, скопіювати до /data Controller,
після live запустити тільки TestDiagnosticParallelReservationIsAtomic і
TestDiagnosticEstimatedBytesAtomicReservation з приватним
NETMONORC_TEST_DSN_FILE=/run/netmonorc/database.dsn. DSN не публікується.
Стенд зупинено, PKI/.local/volumes збережено. Це один Docker host із двома
логічними зонами, не HA/load benchmark чи доказ незалежності фізичних хостів.
[Контракт](../../architecture/fixed-parallel.md), [залишок](../../deployment/yaml-controls.md).
