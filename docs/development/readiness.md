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
R26 додає похідні/context ResourceGroup. R30 підключає одночасне виконання
Monitor catalog на спільних Agent; ACK, assessment і діагностика розділені
за revision/Monitor. Попередня приблизна оцінка відсотків вище не є новим
перерахунком після цієї ітерації.

| Великий залишок | Що визначає завершення |
|---|---|
| Незалежний hot reload Monitor | R30 виконує кілька планів з окремими ACK/replay/lease, REST/React та Assessment; зміни catalog усе ще перезапускають спільні Agent streams; storm/performance вимірювання лишаються |
| Повний placement і replicas | R31 додає sticky leastLoaded/roundRobin, actual telemetry/target policy, persistent cursor і failover на fixed roster; залишаються trusted selectors/failureDomain, per-check overrides, compiler-default; R32 додає same-zone slots/all/any/quorum; CEL values/metrics aggregation відкладена; partial вимкнений |
| Подальший dependency context | R27 закрив окремі Dependency refs/requires DAG/revisioned IR; R28 додав графічний перегляд сторінки; лишився неузгоджений shorthand, автоматичний RCA не додається |
| Agent lifecycle | Enrollment/recovery/revocation, certificate rotation та UI/API керування; поточний trusted fixed roster цього не замінює |
| Приймання й експлуатація | Browser user flows, installer/оновлення, фінальна звірка контрактів, backup/recovery та повторні дослідницькі/навантажувальні експерименти |

Семантика CEL values/metrics для кількох replicas раніше відкладена користувачем;
не можна вигадати усереднення заради формального завершення. Межі й результати
кожної ітерації фіксуються в IMPLEMENTATION_STATUS.md та docs/experiments/.
Точні строки завершення без вимірювання решти етапів не обіцяються.
