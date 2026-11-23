# DiagnosticBinding → parsed Fixed evidence → shared PlacementPolicy

[Контракт, YAML-приклад та межі](../../deployment/declarative-diagnostics.md).
Підстава: три напрями аудиту YAML/CEL, доручені користувачем; записка §2.1–2.4,
DSL §8/§10/§11/§18. Base commit e21cefb9d7c3c8fcecaaaa82e2883cd0a310a59f.

Фінальний owned project `nmo-declarative-final2-20261123`: production образи,
PostgreSQL, HTTPS API, mTLS Agent transport і реальні HTTP probes.
Controller inventory: branch має site=office, dc має site=server; labels не
надходять із Agent Hello. Обидва мають відомі Controller failure domains.

`live.json` підтверджує:

- PUT Monitor з monitor.sttae повертає 422 і зберігає ETag; правильний
  name/coverageState trigger прийнято, точна нова revision досягла healthy;
- production CLI validate читає multi-document YAML PlacementPolicy +
  DiagnosticProfile із ref, inline selector та parsed values condition;
- parsed-serial (maxParallel=1) і parsed-parallel (maxParallel=2) мають
  state=completed, additionalChecks=2 та два physical tasks. Read за ref
  виконується на branch, confirm за selector — на dc;
- linked durable Observation містить depth=0 і ready=true. Умова використовує
  ці типізовані значення та реальний reasonCode=expected_status для запуску
  confirm. Самі values не реконструюються з expectedOutcomes;
- parsed-missing читає відсутнє values.absent: session=inconclusive,
  additionalChecks=1, фізичного confirm немає. Відсутність не стає нулем;
- DELETE використаної PlacementPolicy із коректним If-Match повертає 409;
  regular monitoring лишається healthy, cookie-сесію завершено.

`go.log`/`vet.log`: повні go test ./... та go vet ./... з exit=0.
Нові unit cases перевіряють закритий trigger, exact int64/zero/false/double,
missing values, delivery committed evidence до guard в обох engines,
перетин selector/refs/zones/domain, ref resolution та зміну revision при
оновленні політики. DSN-залежні tests за звичайного запуску можуть бути
пропущені; наведені REST/DB assertions виконані на справжній PostgreSQL.
`build.log` — production Docker/React build; `runtime.json` — образ, base
commit, поточні source hashes і межі. Секретів та приватних ключів немає.

Відтворення з кореня репозиторію (потрібні зібрані образи та вільний prefix):

```sh
python scripts/lab_declarative_diagnostics.py prepare --project nmo-parsed-new --prefix 172.27
python scripts/lab_declarative_diagnostics.py verify --project nmo-parsed-new --prefix 172.27 --output .local/parsed-new/live.json
python scripts/lab_declarative_diagnostics.py down --project nmo-parsed-new --prefix 172.27
```

Інтеграційні спроби виявили та виправили два runtime шляхи: manual start
компілював ref-профіль без зовнішніх політик; DELETE не мав явного SQL guard
для DiagnosticAction refs і повертав 422 замість 409. Helper також виправлено
для чинних observationIds[] та If-Match контрактів. Після перезапуску
середовища тимчасові /tmp логи втрачено; фінальні логи збережено в .local,
а фінальне приймання повторено на свіжому project. Старі стенди та фінальний
зупинено без видалення `.local`, PKI або Docker volumes.

Межі: один Docker host, metadata domains не доводять фізичну незалежність;
нових browser clicks немає. Для action досі replicas=1/pending; multi-replica,
partial та Monitor placement inheritance не додані. Повний monitor/target/
checks/diagnostic CEL context і Alert zone scope залишаються відкритими.
Не заявляється 100% виконання записки або повного старого ТЗ.
