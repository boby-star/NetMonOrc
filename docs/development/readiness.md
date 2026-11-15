# Оцінка залишку роботи

Стан на 16.11.2026 (Asia/Jerusalem). Орієнтир для повного погодженого обсягу:
**приблизно 65–75% готовності, 25–35% роботи залишилось**. Це інженерна оцінка
трудомісткості й ризику за матрицею T01–T16, а не сертифікований coverage або
частка файлів/тестів. Старі відсотки мінімального прототипу не застосовуються.
Не можна отримати 100% лише додаванням інтерфейсів чи проходженням smoke test.

Працює основний наскрізний процес: конфігурація → mTLS Agent → probing →
Observation → зональна оцінка/CEL → обмежена діагностика → alert/webhook.
Є persisted resource catalog, залежні ревізії, REST/CLI/React, fixed placement,
зональні SLA, history, auth/RBAC та відтворюваний приватний Compose stand.
R26 додає похідні/context ResourceGroup у межах описаного runtime.

| Великий залишок | Що визначає завершення |
|---|---|
| Multi-Monitor execution | Кілька одночасних планів на Agent; ACK/replay/lease та оцінки окремих Monitor без взаємного fencing; узгоджені API/діагностика/бюджети |
| Dynamic placement і replicas | leastLoaded/roundRobin, стабільні фізичні slots, all/any/quorum від planned N, контроль відсутніх результатів; partial лишається вимкненим |
| Подальший dependency context | R27 закрив окремі Dependency refs/requires DAG/revisioned IR; лишилися неузгоджений shorthand та графічний перегляд, без автоматичного RCA |
| Agent lifecycle | Enrollment/recovery/revocation, certificate rotation та UI/API керування; поточний trusted fixed roster цього не замінює |
| Приймання й експлуатація | Browser user flows, installer/оновлення, фінальна звірка контрактів, backup/recovery та повторні дослідницькі/навантажувальні експерименти |

Семантика CEL values/metrics для кількох replicas раніше відкладена користувачем;
не можна вигадати усереднення заради формального завершення. Межі й результати
кожної ітерації фіксуються в IMPLEMENTATION_STATUS.md та docs/experiments/.
Точні строки завершення без вимірювання решти етапів не обіцяються.
