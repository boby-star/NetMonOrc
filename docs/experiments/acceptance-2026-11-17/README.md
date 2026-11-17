# Приймання дослідницького профілю — 17.11.2026

Перевірено продовження від remote `4e60ed0`, виправлення `12a149c` та
інтеграцію UI/CLI/harness `34d0353`. Поточна матриця за завантаженою редакцією
записки: [вимоги й залишок](../../planning/minimal-research-acceptance-2026-11-17.md).
R33 enrollment у доступному checkout/history не знайдено; її браузерний
результат не заявляється підтвердженим.

## Реально перевірено

- Усі Go packages, vet, критичні race suites, окремий uncached expiry race.
  Регресії перевіряють повернення гіпотез після expiry, заміну старого доказу
  новим позитивним/негативним та збереження витраченого бюджету.
- Справжній PostgreSQL: webhook summary isolation, browser sessions/CSRF,
  durable budgets, незалежні ACK, placement persistence, durability/fencing.
  Тести працювали в окремій новій БД, не в дослідницькому catalog.
- 11 Python unit tests, syntax checks і відхилення наявного report directory
  до звернення до стенда зі збереженням контрольного файла.
- Production image з pinned dependencies, TLS verification і тимчасовим
  build CA secret; CA не додавався до runtime image.
- Chromium через HTTPS із приватним NSS trust store, без insecure TLS flags:
  login, regular Observation detail, automatic diagnostic session/tasks та
  diagnostic Observation detail, Target/Dependency CRUD, graph, ResourceGroup,
  reference delete guard, revision change/restore, два одночасні Monitor з
  протилежними станами, reload session, logout/revocation. Console exceptions: 0.
- Production CLI із короткоживучим тестовим PAT: diagnostic session → physical
  task → exact Observation, перевірка збережених session/task/Agent IDs та
  snapshots обох Agent. PAT відкликано, локальний context видалено штатним logout.
- HTTP 503 → webhook retry/200; втрата обох Agent → unknown без resolve;
  restart Controller без повторної logical firing; recovery та durable
  resolved retry з тим самим event ID; HTML показав committed delivered.

`checks.json`, журнали, `browser.json` і `notifications.json` містять фактичні
результати. `panel.png` — фрагмент реального графа демонстраційних ресурсів;
drilldown перевіряється browser assertions, а не цим зображенням.
Початкову невдалу DOM-serialization перевірку й невдалі спроби запуску storage
тестів без правильних fixture/чистої БД не зараховано; наведено успішні повтори.

## Порівняльна серія

Усі **15/15** станів і результатів відповідають внесеним faults: 12 підтриманих
одиничних класів та 3 очікувані inconclusive для HTTP+DNS; harness errors: 0.
Static виконав по 6 completed probes, Fixed та Adaptive — по 4.

`series.json`/`series.csv` — сирі результати нового контрольного повтору.
`RESULTS.md` генерується з них, не редагується для поліпшення результатів.
Одна репетиція п'яти faults у Static/Fixed/Adaptive, confirmed profiles,
seed 1711. Expected state не використовується як умова очікування Assessment;
ground truth лишається у зовнішньому harness.

Під час серії було завершено UI-only image build. Static працював із образом
`8a14bb4…`, Adaptive/Fixed — `d56738b…`. Raw metadata містить початковий образ;
точні per-mode IDs записані в `runtime.json`. SHA256 чотирьох виконуваних
бінарних файлів (Controller, Agent, CLI, experiment) **побайтово однакові**
в обох образах — див. `image-binaries.json`. Зміна UI не змінювала алгоритм,
probes, каталог або budgets. Фінальний browser/CLI run — на `d56738b…`.
Дерево під час build/серії містило майбутні коміти; raw git_dirty збережено.

Це regression acceptance на одному фізичному хості, не статистична гарантія
точності/latency та не доказ переваги Adaptive над Fixed. Мережеві байти не
вимірювались. Результати старих 30+9 runs збережені окремо й не називаються
новим запуском. N=3 physical replicas, реальний OS suspend, production
storm/HA і lifecycle сертифікатів цим запуском не перевірялися.

## Відтворення

З нового owned project, не поверх наявних користувачів/PKI:

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project YOUR_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_PROJECT --prefix YOUR_PREFIX --ui-port YOUR_FREE_PORT
python3 scripts/lab_browser.py --project YOUR_PROJECT --prefix YOUR_PREFIX --multi-monitor \
  --output .local/NEW_DIRECTORY/browser.json --screenshot .local/NEW_DIRECTORY/panel.png
python3 scripts/lab_notifications.py --project YOUR_PROJECT --prefix YOUR_PREFIX \
  --output .local/NEW_DIRECTORY/notifications.json
python3 scripts/lab_experiment.py --project YOUR_PROJECT --prefix YOUR_PREFIX \
  --policy confirmed --repetitions 1 --seed 1711 --modes static,fixed,adaptive \
  --scenarios http,dns,tcp,office-path,multiple --output .local/NEW_DIRECTORY/series.json
python3 scripts/experiment_report.py .local/NEW_DIRECTORY/series.json \
  --output .local/NEW_DIRECTORY/RESULTS.md
python3 scripts/lab.py down --project YOUR_PROJECT --prefix YOUR_PREFIX
```

За корпоративного TLS proxy використовуйте документований build-ca override,
не вимикайте TLS checks. Browser harness потребує Chromium, certutil і Node 24+.
Секрети надходять через пам'ять/stdin; CA keys/DSN/PAT не входять до evidence.
Експеримент тепер експортує звіти до власного `series-report/` і відмовляється
перезаписувати наявний каталог. Старий спільний `.local/lab-report` перед
завершенням поточного процесу збережено окремо; його початкові файли поновлено
після копіювання нового export. Docker volumes не видалялись.
