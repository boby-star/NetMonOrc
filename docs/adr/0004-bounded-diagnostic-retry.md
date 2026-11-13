# ADR 0004: явний technical retry діагностичної активації

Статус: реалізовано; критичні перевірки та натурні Fixed/Adaptive пройдено.

Користувач раніше погодив DiagnosticAction.retry.maxAttempts і облік усіх
фізичних спроб у чинному budget. Реалізовано цей об'єкт без нових routes,
runner kinds, retry delay чи автоматичного fallback. DSL §11.4.1 фіксує
типи та семантику. За дорученням самостійно обирати рекомендовані рішення
обрано maxAttempts для однієї запланованої action × Agent активації,
включно з первинною спробою. Наявність об'єкта вимагає явного числа 1…50;
без нього повтору немає. Межа 50 відповідає максимальному physical budget.

Retry — продовження поточної активації на тому самому Agent. Повторне
явне Fixed step або confirmation probe має окремий лічильник активації,
проте всі вони витрачають той самий episode budget. Така межа дозволяє
відділити повтор через зламаний інструмент від нового доказу для confirmation.
Completed failure і unknown не повторюються цим механізмом. Якщо потрібні
інші докази, їх має дозволяти профіль; повтор не змінює Gain/Cost.

Перед кожною спробою перевіряються cancellation, час на повний probe з
orchestration allowance, checks, estimated bytes та eligibility. Кожна
спроба створює новий task ID, durable reservation і номер attemptNo.
Технічний результат не відсіює гіпотези й не додає proof/domain. Fixed
передає залежним умовам результат завершеного ланцюга; вичерпана technical
помилка лишає крок недостатньо підтвердженим. Adaptive не отримує права
повторного вибору action через новий Run з тим самим Ledger.

Runtime підтримує одну послідовну діагностичну lane на Agent. Свіжість
сесії й generation перевіряються повторно в transport/storage, а Agent
перевіряє lease/deadline/allowlist для кожного нового task. Старий task ID
не запускається вдруге. Crash Controller зберігає reservations та завершує
сесію inconclusive; активний retry chain не відновлюється автоматично.

Натурний smoke використовує лише operator-selected test executable:
один startup реального exporter завершується до readiness, далі запускається
звичайний Blackbox 0.27.0. Це fixture поза YAML і продуктовим кодом, а не
shell engine чи підставлений service outcome. Реальні mTLS/Agent/spool/PG
зберігають execution_failed, потім completed/failure. Друга подія доводить
відмову сервісу, перша — тільки технічну проблему.
