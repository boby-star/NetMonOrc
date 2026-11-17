# Перегляд окремого спостереження (R18)

`GET /api/v1/observations/{id}` читає один збережений нормалізований результат.
Потрібна cookie-сесія або PAT зі scope `resources:read`. Bootstrap token
не надає доступу до історичних доказів. Query parameters не підтримуються.

Відповідь має той самий Observation DTO, що й історія Monitor: ідентифікатори
перевірки, агента й зони, конфігураційна ревізія, executionStatus та probeOutcome,
reasonCode, час початку/завершення/приймання, durationMs і дозволені scalar values.
Час приймання береться з PostgreSQL. Для діагностичних probes повертаються
збережені task ID та session ID, встановлений через зв'язок із фізичною задачею.
Відсутні failureDomain/isReplay не вигадуються. Сире HTTP body, runner config,
credentials та приватні ключі не входять до проєкції відповіді.

Читання не обмежене поточною ревізією: старі результати потрібні для аналізу
інцидентів, але не повертаються цим запитом у live Assessment. Втрачене через
retention спостереження дає 404. Неприпустимий ID або query — 400, інший метод —
405, брак scope — 403, зайнятий спільний read slot — 429 з Retry-After,
недоступність сховища — 503. Запит має timeout 3s, відповіді — Cache-Control: no-store.

## CLI та React

```sh
netmonorc get --context-file /private/context.json observations OBSERVATION_ID
```

CLI читає точний ID без pagination/filter/task flags і повертає серверний JSON.
PAT зберігається у приватному context file, не передається аргументом команди.
У React відкрийте «Історія перевірок» → «Відкрити доказ». Деталі завантажуються
окремим запитом; закриття або вибір іншого запису скасовує попередній запит.
Переглядач показує збережені значення, не обчислює стан ресурсу в браузері.
JavaScript number не призначений для точного відображення int64 поза safe integer
range; для машинного аналізу точного JSON використовуйте CLI.

## Критичні перевірки

`python3 scripts/identity_smoke.py` перевіряє реальні PostgreSQL/HTTPS: стару
ревізію, execution failure окремо від негативного probe, збереження false/zero,
diagnostic task/session provenance, 404/400/405 та відхилення PAT іншого scope.
`scripts/lab_sessions.py` перевіряє відповідність detail запису в історії та
читання фактичного результату через CLI на стенді з двома агентами. React
перевіряється production build; браузерні кліки цим сценарієм не перевіряються.

## Зв'язок із фізичною діагностикою

«Діагностичні сесії» → «Відкрити» → таблиця «Фізичні задачі Agent» →
«Відкрити доказ» використовує той самий серверний Observation DTO.
Посилання доступне зі scope resources:read; відсутній/видалений retention
доказ показує помилку API без підміни новішим результатом. Деталі містять
збережені session/task IDs для перевірки походження.

`lab_browser.py` тепер створює справжній HTTP fault, чекає автоматичну
діагностику, повертає healthy і перевіряє обидва drilldown у Chromium
зі справжнім HTTPS Controller, PostgreSQL та двома Agent.
