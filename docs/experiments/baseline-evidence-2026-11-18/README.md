# Початкові регулярні докази діагностичного висновку

Вимоги §2.1–2.3 записки: використання відповідних регулярних спостережень
до додаткових запусків, сталі ідентифікатори, походження та пояснюваність
висновку. Раніше baseline впливав на гіпотези, але не зберігався як окремий
вхід звіту; export збирав переважно Observation діагностичних tasks.

Тепер нові terminal reports зберігають `baselineEvidence` з відповідністю
каталожної дії фактичній перевірці, Agent/zone/domain, revision, time/TTL та
execution/outcome. API/React показують цей вхід; CLI/React export додають
регулярні Observation, перевіряючи саме regular provenance. Невідомий legacy
набір не видається за порожній. [Контракт](../../deployment/diagnostic-export.md).

## Підтверджений запуск

Окремий свіжий project `nmo-baseline-final-20261118`, приватні мережі,
mTLS Agent, реальні Blackbox probes, PostgreSQL і HTTPS API:

- HTTP-дія еквівалентна регулярній перевірці `http` з office/branch;
- adaptive висновок `responding` зі збереженого фізичного Observation;
- session=completed, budget additionalChecks=0, decisions=[], physical tasks=0;
- baseline містить actual checkId, actionId, Agent/zone/domain та revision;
- finishedAt і revision збіглися з GET точного регулярного Observation;
- production `netmonorc diagnostics export` отримав один Observation без tasks;
- complete=true та baselineEvidenceKnown=true; PAT відкликано, logout пройшов.

`live.json` містить результат без облікових даних. `runtime.json` — image ID,
base commit та SHA256 прийнятих файлів; image зібрано з робочого коду до коміту.
`go.log` — всі Go packages; окремий PostgreSQL test DSN не задавався, реальна
БД перевірена натурним сценарієм. `race.log` — diagnostics/apiclient; vet
змінених Go packages пройшов. Production image включає успішну збірку React.
Нових browser clicks не виконувалося.

Цільові регресії перевірили zero-attempt provenance, safe projection,
legacy/known-empty, dedup одного baseline під різними action aliases,
retention 404 і відмову експорту для чужих Monitor/revision/Agent/check/zone/
time/outcome або diagnostic замість regular. Раніші task-export перевірки
також пройшли.

Перший запуск сценарію зупинився на створенні тестового профілю: fixture
не містив обов’язкового `adaptive: {}`. Після виправлення повний сценарій
повторено на іншому свіжому project; перша спроба не є доказом приймання.
Обидва стенди зупинено; `.local`, PKI та named volumes збережено.

## Відтворення

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_baseline_evidence.py --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_DIRECTORY/live.json
python3 scripts/lab.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Для середовища з власним CA застосовується наявний deployment/build-ca.compose.yaml.
Сценарій вимагає project без користувачів та нового output, не стирає існуючих
даних. Тимчасовий приватний CLI context лишається у config volume з уже
відкликаним PAT. Пакет — послідовне читання, не атомарний backup. Baseline
означає вхід сесії; не кожен вхід обов’язково залишився придатним до її кінця.
