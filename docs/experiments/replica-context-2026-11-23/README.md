# Replicas та повний діагностичний CEL-контекст — приймання

[Контракт і межі](../../deployment/replica-context.md),
[публічний YAML](../../../examples/diagnostics/replicated-context.yaml).
Base commit bf52fd8652ee55dae9e3dc957201fbc3ab885f0d.
Джерело записки: SHA256
910615e8281e4978ffd1b1fe30cafb163b05b14057009e4cc7b41522bd0fd308, §2.1–2.4.

Owned project `nmo-replica-context-20261123`: production Controller/Agent/CLI,
PostgreSQL, HTTPS/mTLS, реальні probes й приватні office/server мережі.
Controller labels/domains задаються inventory; Target має exact integer
attribute serial=9007199254740993; Monitor label environment=research.

`live.json` містить шість завершених сценаріїв:

| Сценарій | Результат | Фізичні витрати |
|---|---|---|
| fixed-serial, maxParallel=1 | completed; два read на різних Agent, потім confirm | 3 tasks/checks |
| fixed-parallel, maxParallel=2 | completed; read реально перекриваються, confirm після обох | 3 tasks/checks |
| adaptive-pair | completed після всього набору, хоча перший результат уже розрізняє класи | 2 tasks/checks |
| replica-budget, maxChecks=1 для N=2 | inconclusive, жодного часткового запуску | 0 tasks/checks |
| cancel | cancelled; два accepted tasks мають terminal observations | 2 незворотні reservations/checks |
| automatic-trigger | HTTP 503 → typed labels/attributes/optional-score trigger → completed broken | 2 tasks/checks |

Fixed condition у реальному запуску прочитала monitor.name/labels,
target.attributes, bare checks/zone, явні zones.server.checks, attempts,
remainingChecks, elapsedSeconds та окремі replicaResults.*.values. Дані
відрізняються за походженням: regular checks не замінені diagnostic results.
Автоматичний trigger окремо виконався з !has(monitor.weightedScore) за двох
зон, без вигаданого глобального середнього. Після fault target відновлено,
regular monitoring повернувся до healthy, cookie-сесію завершено.

Хронологія raw Observations підтверджує послідовність за maxParallel=1,
перекриття за maxParallel=2 та barrier залежного кроку. Обидва read мають той
самий logical stepId і різні task/Agent/Observation IDs. Це не дублікати/replay.

example-validation.json — production CLI validate публічного multi-document
YAML зі спільним resolver. go.log/vet.log — повні suites із exit=0;
critical.log — додаткові цільові cases для replicas і context: конфлікт
джерел не дає completed; точний int64, TTL окремої replica, pending/inherited
placement та static check/zone refs. DSN-залежні tests можуть бути пропущені
за звичайного Go запуску; наведений live trial працював із реальною БД.
build.log — production Docker/React build, runtime.json — образ, base commit
і source hashes. Секрети не експортувалися.

```sh
python scripts/lab_replica_context.py prepare --project nmo-replicas-new --prefix 172.27
python scripts/lab_replica_context.py verify --project nmo-replicas-new --prefix 172.27 --output .local/replicas-new/live.json
python scripts/lab_replica_context.py down --project nmo-replicas-new --prefix 172.27
```

Нове output path обов'язкове; helper відмовляється використовувати project
із чужими/попередніми users. Проміжний packet має status=in_progress і не є
успішним прийманням; фінальний має status=passed. Стенд зупинено без видалення
.local, PKI або Docker volumes.

Межі: один Docker host, різні мережеві перспективи; metadata failure domains
не доводять фізичну незалежність. Нових browser clicks, HA/load або всіх
restart/partition trials немає. N>1 values не усереднюються; multi-zone score
без нормативних ваг відсутній; partial placement не додано. Alert zone scope
та regular multi-replica numeric aggregation — поза цими діагностичними
функціями. Повна готовність усього старого ТЗ не заявляється.
