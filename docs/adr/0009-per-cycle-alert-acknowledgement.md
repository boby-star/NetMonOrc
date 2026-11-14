# ADR 0009: Окремий receipt поточного циклу алерта

Статус: локальне рішення реалізації дорученого acknowledgement; конкурентні
зміни захищено за погодженим правилом Q07. Математика Alert FSM не змінюється.

Logical Alert UUID переживає декілька циклів. Тому POST за одним ID без
precondition міг би підтвердити нове спрацювання зі старої вкладки. GET Alert
тепер повертає strong ETag representation; POST вимагає If-Match. Порівняння
і запис серіалізуються з EvaluateAlert через наявний row lock.

Receipt зберігається окремо за alert ID/cycle identity із firing timestamp;
не змінює FSM або outbox. AcknowledgedAt — поточна projection, ledger зберігає
історію. Новий cycle скидає projection receipt. Підтвердження лише firing;
first receipt wins, повтор з однаковою note не створює нового запису.

Примітка та actor фіксуються durable, але не включаються в webhook чи Alert
DTO. CLI зберігає серверний opaque ETag і передає його без обчислення у клієнті.
Не додаються DSL поля, альтернативні HTTP routes або suppression semantics.
