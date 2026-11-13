# ADR 0002: час probe окремо від доставлення задачі

Статус: реалізовано; окремий silent-DNS прогін перевіряє поведінку.

Попередній route-withdrawal пілот показав execution_failed замість
завершеного негативного probe: Engine давав усій задачі check.timeout,
Agent віднімав 5s clock uncertainty, а Blackbox startup і повернення
результату також займали цей час. Для timeout=10s фізичний DNS міг мати
лише близько 5s; це не дозволяло дочекатися його власного timeout.

Контролер резервує всередині незмінного budget.maxDuration додатковий
TaskOverhead=20s: 5s clock bound, до 5s startup та решту для реєстрації,
доставлення й durable result. Це внутрішня межа orchestration, не нове
YAML-поле і не збільшення мережевого timeout. Pure Engine має zero default
для replay/інших adapters; admissibility та context враховують overhead.
Контекст завжди обмежений глобальним deadline. Реальні додаткові затримки
не обіцяються меншими за цей запас: перевищення дає inconclusive.

Перед DiagnosticTask надсилається новий ConfigAssignment тієї самої revision
із чинним 30s lease. Agent приймає його до task, віднімає clock uncertainty
та обмежує task deadline lease. Він перевіряє, що лишився час на цілий
check.timeout і bounded startup, до підтвердження receipt. Уже запущений
probe не отримує необмеженого подовження; disconnect/expiry його скасовує.

У мінімальному live профілі one-shot timeout обмежено 100ms..15s, щоб
вписатись у підтверджений lease зі startup margin. DSL/compiler зберігають
загальний діапазон 100ms..60s; довші live one-shot checks явно відхиляє
Coordinator. Недостатній часовий залишок Agent також відхиляє, а не запускає
завідомо скорочений probe. Довгі regular checks лишаються обмежені lease;
повний протокол довгих задач і BOOTTIME/suspend — поза цим етапом.

Для перевірки fixture підтримує dns-drop: UDP query приймається, але жодна
DNS-відповідь/ICMP-відмова не надсилається. Це реальна мовчазна втрата
відповіді, не підставлений Outcome. Blackbox має завершити власний 10s
network timeout і передати completed/failure; execution error не
перетворюється на відмову ресурсу. Усі режими мають ті самі бюджети й запас.
