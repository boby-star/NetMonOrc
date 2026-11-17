# Спільна місткість Agent для діагностики

Підстава: §2.3 «Чернов С. записка v1.docx» (поточна редакція SHA256
`df5f9d1f6154a65713b48455d58a69c17e961f2eabab61e359f8d3610d184d8c`):
«Перед призначенням нового завдання резервуються потрібні ресурси»;
«Регулярний моніторинг і діагностика мають розділяти доступну місткість агентів».
Ця зміна реалізує ці вимоги в наявному двослотовому Agent: один regular,
один diagnostic. Формули gain/cost, all/any/quorum та формат YAML незмінні.

## Резервування

Усі Monitor одного Controller ділять один diagnostic admission slot кожного
Agent. Допустимість цілі/runner/placement та звичайні бюджетні перевірки
зберігаються. AdmissionRunner резервує слот **до** обліку фізичної спроби.
Snapshot active telemetry не підміняє це атомарне резервування.

Якщо обраний Agent уже виконує діагностику іншого Monitor, нова сесія може
чекати до останнього допустимого початку probe: спільний deadline мінус
probe timeout та task overhead. Очікування входить до загальної тривалості
епізоду, не створює Task ID і не збільшує checks/cost/estimatedBytes. За
скасування/неможливості admission сесія завершується inconclusive з причиною;
сам факт зайнятості Agent не є доказом відмови ресурсу.

Після очікування повторно перевіряється expiry початкових доказів. Якщо
вони застаріли, ресурс звільняється, hypotheses/support і кандидат
перераховуються до запуску. Revision change скасовує старий runtime за
наявним fencing; протерміноване або відсутнє ACK/heartbeat забороняє admission.

Явний technical retry утримує той самий admission slot у межах свого ланцюжка;
кожна фізична спроба, як раніше, має новий task ID та витрачає бюджет. Release
ідемпотентний; застарілий result іншого task не звільняє поточний слот.

## Завершення та невизначеність

Збережений result або негативний TaskReceipt підтверджує завершення поточного
віддаленого task. Якщо запуск завершився cancel/timeout/втратою відповіді,
слот консервативно утримується до terminal result або абсолютного task deadline.
Сам запит TaskCancel не є підтвердженням зупинки. Новий stream/generation не
дозволяє перекрити невизначений попередній task на тому самому Agent.

Agent завершує фізичний probe, закриває власний Blackbox і звільняє busy/active
перед публікацією Observation у spool. Завершення старого worker не може вдруге
скинути busy прапорець нового task. Доставка/ACK/replay лишаються durable;
виконана спроба не повертається до бюджету через мережеву помилку.

## Межі

Admission є in-memory координацією одного Controller, не distributed lock
для HA. Повний reload досі розриває streams; порядок чекання не гарантує FIFO.
Queue fairness і навантажувальна продуктивність не заявляються перевіреними.
Якщо жодного допустимого онлайн Agent немає, лишається чинний inconclusive;
admission не обходить eligibility і не обирає нову зону замість потрібної.

Цільові тести покривають contention між Monitor, незалежні Agent, cancellation,
release після result/deadline та відсутність budget charge за саме очікування
в Static/Fixed/Adaptive. Окремо перевірено перебудову baseline після queue wait.
Натурний сценарій: `scripts/lab_admission.py` на новому prepared/up project.

Результат натурного запуску: [admission-2026-11-17](../experiments/admission-2026-11-17/README.md).
