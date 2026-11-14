# Перемикання діагностики через Monitor (R9)

Monitor.diagnostics.enabled можна змінити одиночним PUT або config/apply.
Доступність локального DiagnosticProfile і дозвіл запускати діагностику тепер
розділені: --diagnostic-profile завантажує валідований каталог, enabled визначає,
чи створює Controller coordinator. Сам факт наявності файла не запускає задач.

При enabled=false регулярні checks та Assessment працюють. Новий runtime не
створює coordinator; diagnostic runner додатково відхиляє виконання для disabled
плану. Старий runtime/задачі проходять наявний cancellation і graceful shutdown,
їхні результати залишаються історією своєї ревізії. Після ACK нового плану нові
автоматичні діагностичні сесії для цього Monitor не стартують.

Повторне enabled=true потребує доступного профілю з відповідним profileRef.
Registry розгортає його в effective revision, Controller завантажує план і
поновлює автоматичні тригери. Зміни binding змінюють effective revision;
спостереження інших ревізій не використовуються як поточні докази.
Incident budget/retry/confirmation і правила planner не змінені.

Дослідницький --diagnostic-comparison-static як і раніше вимагає enabled,
розгорнутий профіль: вимкнення binding у цьому режимі відхиляється. Public
manual diagnostics API ще не реалізований; enabled=false не отримує bypass
через доступність каталогу. Profile CRUD та багатопрофільний каталог — наступні
етапи; цей зріз використовує один встановлений локальний profile.

Перевірено Compose-сценарій: CLI apply --wait вимикає binding, сервіс отримує
контрольований HTTP fault, Assessment стає down, кількість diagnostic runs не
зростає. Після recovery і повторного enabled=true новий fault створює нову
автоматичну сесію. Потім сервіс повертається до healthy. Це перевірка запусків,
не твердження про правильність висновку RCA для короткочасного fault.
