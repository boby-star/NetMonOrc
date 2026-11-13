# CEL-правила оцінювання

Реалізовано inline `assessment.rules` з normative DSL §9.5: ID, priority,
when, then.state та then.reason. Компілятор сортує priority DESC / ID ASC,
валідує тип bool і зупиняється на першому істинному правилі.
Текст і параметри правил входять до effective revision.

Controller створює Assessor один раз при завантаженні конфігурації.
Щосекундне оцінювання використовує готові CEL-програми. Вирази обмежені
4096 bytes, 1024 cost units і загальним deadline 25ms на зону.
Немає мережевих/файлових функцій, shell або SQL у CEL.

Поточний типізований контекст:

- checks[id].outcome, executionStatus, reasonCode, ageSeconds;
- checks[id].metrics.httpStatus — integer, лише за виконаного HTTP probe
  з відомим response status; відсутній статус не підставляється як нуль;
- checks[id].values[name] — отриманий string/integer/float/boolean із regex extraction;
  відсутнє значення не підставляється як false/zero;
- zone.name, coverage, labels; target.address, type, networkZone,
  criticality, labels. Відсутні labels мають документований порожній default.

Рекурсивний zone.state не входить до типу; monitor.state не доступний
для AssessmentRule. Некоректні поля й небулеві вирази відхиляються.
Додаткові metrics, зокрема latencyMs, поки не реалізовані.
Порожні або вигадані значення за них не надаються. Наприклад, відсутній
latencyMs можна перевірити через `has(...)`; прямий доступ дає evaluation error.

Після правил застосовуються інваріанти: required failure завжди down,
неузгоджені за часом результати не дають нового комплексного висновку,
healthy потребує required successes і достатнього Q. Помилка вищого
правила не є звичайним false: навіть нижче healthy-правило не знімає
невизначеність. У зональній оцінці зберігаються ruleErrors та reason.

Явне правило може визначити degraded/down за часткового coverage,
якщо наявний придатний узгоджений контекст. Втрата Agent без придатних
доказів не дозволяє такому правилу створити підтверджений down.

Приклад `examples/cel.yaml` навмисно допускає HTTP 200 і 503 як
спостережувані probe outcomes. CEL окремо класифікує status>=500 як
відмову ресурсу. Це відділяє успішне отримання відповіді від оцінки ресурсу.

```bash
go build -o .local/netmonorc-controller ./cmd/netmonorc-controller
go build -o .local/netmonorc-agent ./cmd/netmonorc-agent
python3 scripts/distributed_smoke.py --cel
```

Сценарій перевіряє правила через реальні Controller/Agent, mTLS,
PostgreSQL, replay, HTTP failure/recovery та TTL. Прямі unit-тести
перевіряють priority, missing metrics, required failure override та
заборонені рекурсивні контексти. Нормативні файли не змінені.
