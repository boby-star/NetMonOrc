# Реальний повтор після технічної помилки exporter

Source `fc0d7bd`, чисте дерево на початку, зібрані з цієї ревізії Controller
та Agent. Три незалежні процесні прогони з окремими тимчасовими PostgreSQL:
Static, Fixed і Adaptive. Raw result JSON та stdout log збережено; SHA256SUMS
рахується від точних байтів. Docker image також зібраний Go 1.25.4;
натурні прогони використовували host binaries, не цей контейнерний image.

Контрольована несправність інструмента: operator-selected private wrapper
один раз завершується до готовності diagnostic Blackbox, далі через execv
запускає реальний Blackbox 0.27.0. Регулярний native HTTP моніторинг не
переривається цією fixture. Контрольований сервіс повертає HTTP 503.
Негативний probe не підставляється й не маскується під technical error.

| Режим | Зарезервовані спроби | Execution failed | Completed probes | Висновок |
|---|---:|---:|---:|---|
| Adaptive | 2 | 1 | 1 | service-failure |
| Fixed | 3 | 1 | 2 | service-failure |
| Static | 3 | 1 | 2 | service-failure |

Для inspect-server усі режими зберегли два різні physical task ID:
attemptNo=1 — execution_failed/unknown, attemptNo=2 — completed/failure.
Негативний probe автоматично втретє не повторювався. У Fixed/Static
окремий inspect-office — самостійна дія зі своїм ordinal 1, не retry.
Adaptive отримав service-failure лише після придатного server proof.

Кожен прогін також перевірив replay/dedup після reconnect, foreign CA
rejection, Agent TTL→unknown, Controller restart без поновлення episode,
crash активної наступної сесії→inconclusive з однією збереженою reservation.
Обидва storage integration тести виконувалися з реальною БД: generation
fencing/ID conflicts та budgets/diagnostic isolation. Логи містять PASS
саме виконаних перевірок, не лише результат компіляції.

Це перевірка інтеграції одного повтору кожного режиму на одному хості.
Вона не є статистичним порівнянням latency або незалежних зон. Окремі
unit-тести перевіряють exhausted budgets, cancel/deadline, absent retry,
completed unknown, незалежність confirmation domains і revision hash.
OS suspend, довгі offline queues та довільні технічні відмови цим пілотом
не перевірені. Wrapper не входить до продуктової логіки чи DSL.

Відтворення: команда в metadata.json, MODE = adaptive/fixed/static.
[ADR 0004](../../adr/0004-bounded-diagnostic-retry.md).
