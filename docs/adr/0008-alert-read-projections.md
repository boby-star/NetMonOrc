# ADR 0008: Транзакційні проєкції алертів

Статус: локальне рішення реалізації доручених REST/React інтерфейсів.
HTTP маршрути та математична семантика Alert FSM не змінюються.

Наявна FSM payload містить стан та службові timestamps, але не всі metadata
публічного Alert. Поточний Assessment не є достовірним джерелом первинної
причини старого firing. Тому під тим самим row lock EvaluateAlert записує
окремий typed api_view і стабільний logical UUID. State/event/outbox/view
комітяться разом. Firing і resolved evidence зберігаються до наступного transition; stale inputs
не змінюють проєкцію. Legacy rows не отримують вигаданих historical facts.

API читає окремі sanitized DTO. History береться зі збережених immutable
transitions, включно з suppressed, а delivery summaries — з outbox/attempts
поточної endpoint revision. Read model не керує FSM, retry чи Assessment.
Generic Page історії конкретизовано в OpenAPI як PageAlertHistory.

Name-based logical key поточного дослідницького runtime лишається чинним.
Міграція до нормативного UID-based AlertRule/Monitor lifecycle та zone scope
не приховується за новим UUID і потребує наступного розширення registry.
