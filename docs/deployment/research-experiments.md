# Порівняння на живому стенді

Спочатку [зіберіть і запустіть стенд](research-compose.md). Далі:

```bash
python3 scripts/lab_experiment.py --repetitions 1 --output .local/pilot.json
```

Результат: JSON із сирими assessment/diagnostic/task records, плоский CSV,
summary та копія HTML-звіту. Наявні результати не перезаписуються. Скрипт
не зупиняє стенд автоматично, але повертає fixture до healthy. Завершивши,
запустіть `python3 scripts/lab.py down` для зупинки зі збереженням volumes.

Сценарії: HTTP 503, DNS NXDOMAIN, закритий TCP-port, недоступність VIP лише
з office namespace та множинна HTTP+DNS несправність. Протокольні відмови
глобальні; зональна відмова не розриває mTLS. Усі сценарії виконуються для
Static, Fixed і Adaptive. Порядок режимів/сценаріїв перемішується за явним
seed, щоб не мати одного постійного порядку прогріву. Це не усуває всі
ефекти спільного хоста чи попередніх запусків.

Controller сам оцінює ресурс і запускає діагностику. Ground truth створює
зовнішній harness через file control fixture/маршрут і зберігає лише у звіті.
Планувальник не отримує ані назви сценарію, ані правильного класу.

Перед порівнянням чистий Go compiler перевіряє однаковість нормалізованих
action catalogs, hypotheses і budgets профілів. Fixed задає природну
послідовність DNS → TCP → HTTP із розгалуженням за наслідком DNS; Static
виконує весь каталог. Для всіх режимів той самий Agent task transport,
Blackbox і послідовне виконання. Нової YAML-версії чи mode немає.

## Що означають показники

- `state_correct` порівнює отриманий steady-state Assessment із truth.
  Семпл береться після нових observations усіх шести check/source pairs
  і наступного Assessment. Очікуваний state не є умовою polling.
- `diagnosis_correct`: для одиничної несправності потрібен правильний
  клас; для multiple — inconclusive без одного вигаданого класу.
- `duration_ms`: монотонний час Engine.Run, включно з task dispatch,
  probe та durable result. Не включає час до автоматичного trigger.
- `assessment_sample_after_control_ms`: час від завершення команди
  внесення відмови до семплу оцінки після свіжих доказів. Це не точна
  latency першого виявлення; сюди входять період checks і polling.
- `reserved_checks` — витрата durable budget, `probe_observations` —
  отримані результати, `completed_probes` — підтверджені завершені probes,
  `execution_failures` — технічні невдачі. Reservation/failed execution
  не є доказом, що мережеве вимірювання фізично завершилося.

Трафік у байтах/пакетах не вимірюється; costUnits не можна так трактувати.
Startup окремого Blackbox для diagnostic task входить до duration для всіх
режимів. Healthy з повним coverage відділяє епізоди, бюджети не обнуляються
через новий observation чи зміну mode. Harness-помилка також лишається у
наборі з status=harness_error; вона не перетворюється на успіх.

Один повтор — smoke/pilot, не статистичний доказ. Для дослідження потрібні
повторення, перевірка truth і ресурсів стенда, розподіли часу, inconclusive
та хибних висновків. Скрипт підтримує 1..30 повторів; заплануйте кількість
і методику до порівняння. Не обіцяйте переваги Adaptive до аналізу даних.

```bash
python3 scripts/lab_experiment.py --repetitions 10 --seed 17 --output .local/repeated.json
```

Можна звузити контрольний запуск: `--modes adaptive --scenarios office-path,http`.
Кожний режим починається з fresh healthy обох Agent. Для іншого project
передайте ті самі --project/--prefix, що під час prepare/up.

## Відкритий недолік методу

Closed-catalog early-stop може підтвердити DNS-class за одночасної DNS+HTTP
відмови, не перевіривши HTTP. Це стосується й Fixed DAG, який після DNS
не проходить наступну гілку. Такі рядки зберігаються як хибні висновки,
а не вилучаються. Потрібні підтверджувальні перевірки чи консервативна межа
класифікації. Поточний прототип не можна вважати завершеним лише тому,
що одиничні сценарії дали очікувані класи.
