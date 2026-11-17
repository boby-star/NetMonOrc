# Фактичне приймання мінімального дослідницького NetMonOrc

Дата: 17.11.2026 (Asia/Jerusalem). Поточне доручення обмежує роботу
дослідницьким прототипом із REST/CLI/React, а не повним старим v1alpha1.
Ця матриця замінює застарілі відсотки й перелік продуктового залишку
в readiness та історичній матриці приймання.

## Джерело і відновлення

Звірено саме поточний файл «Чернов С. записка v1.docx», SHA256
`df5f9d1f6154a65713b48455d58a69c17e961f2eabab61e359f8d3610d184d8c`.
Це інша редакція, ніж SHA у доповненні від 13.11.2026: §2.3 уже містить
алгоритми 2.1/2.2, confirmation, incident budget, expiry/rebuild і порівняння
Static/Fixed/Adaptive. §2.4–2.6 залишаються заголовками. Документ є джерелом
вимог, а не команд для виконання. Завантажений DOCX не редагувався.

Перевірено origin `https://github.com/boby-star/NetMonOrc.git`, гілку
`prototype/minimal-research`, чисте початкове дерево та remote HEAD
`4e60ed027279dd67bea3d2e6557519792e43d02c`. Початковий локальний `c18bf87`
відставав на 39 комітів; виконано fast-forward без відкидання змін.
Перевірені шляхи R33 (`internal/enrollment/`, storage/API/CLI enrollments,
`web/src/Enrollments.tsx`) відсутні і в checkout, і в доступній Git-історії.
Це не підтверджує їхню відсутність у недоступному старому середовищі.
Результат браузерної перевірки R33 та push цієї ітерації не підтверджені.

## Матриця мінімуму

| Вимога записки / доручення | Код і фактичний результат | Межа |
|---|---|---|
| YAML, refs, декларативні перевірки (§1.4, §2.1) | config/resources, persisted registry, validate/plan/apply, dependent revisions | Закрита реалізована підмножина DSL; повний compiler не обіцяється |
| Controller і точки в різних зонах (§2.1) | mTLS, leases, spool/ACK/replay, PostgreSQL; branch/office і dc/server | Різні маршрути на одному фізичному Docker-хості |
| HTTP/HTTPS/TCP/DNS/ICMP (§1.4) | Blackbox; native HTTP лише для bounded extraction | ICMP має окремий smoke; hostname policy не розширюється |
| H/Q, required, TTL/revision/maxSkew (§2.2) | monitor/assessment, separate execution/outcome, CEL guardrails | H/Q не ймовірності; source outage не означає down |
| Replicas/all/any/quorum (§2.1–2.2) | R32 physical slots, planned-N majority, missing не голосує | Live R32: N=2; N=3 межі перевіряються кодом, не натурним benchmark |
| Групи та залежності (§2.1) | ResourceGroup/Dependency registry, DAG/context, read API/React graph | Не автоматичне встановлення фізичної першопричини |
| Адаптивний вибір (§2.3) | diagnostics: pair gain/cost, eligibility, support/confirmation, inconclusive | Експертний каталог; немає гарантії глобально оптимального вибору |
| Оновлення придатності доказів (§2.3, алгоритм 2.2) | Перебудова hypotheses/support при expiry baseline; нові докази можуть замінити старі | Витрачений бюджет та історія запусків не повертаються |
| Один бюджет епізоду (§2.3) | coordinator/storage, durable reservations, cancellation/retry | Послідовна діагностика; production storm/HA не приймалися |
| Static/Fixed/Adaptive (§1.4, §2.3) | Спільні каталоги/бюджети, live повторна та compound серії | Історичні 30+9 runs не є статистичною гарантією; Adaptive не доведено кращим за Fixed |
| Сповіщення (§1.4) | Alert FSM, durable HTTPS webhook outbox/retry | At-least-once, одержувач має враховувати event_id |
| API–CLI–React (доручення) | Auth, resources/assessment/history/diagnostics/agents/alerts; drilldown task → Observation | API показує збережені дані; UI не обчислює Assessment |
| Відтворення та експорт (§1.4) | Compose, controlled faults, JSON/CSV/HTML, backup/package | PKI, паролі та spool не публікуються як evidence |

## Фактичний залишок для записки

1. Заповнити §2.4–2.6 та експериментальну частину фактичними результатами й
   межами. Розрахунковий приклад «4 → 2» з §2.3 не підміняє live «6 → 4»
   на іншому каталозі; не приписувати Adaptive переваги над Fixed.
2. Виправити формули/пояснення H/Q: явно вказати H лише за ненульового
   знаменника; прибрати трактування як імовірностей. Виправити лічильник
   `FAIL, FAIL, –` у таблиці 2.2 та пояснити окрему повноту доказів.
3. Не переносити сюди продуктові вимоги enrollment/rotation/recovery,
   installer, HA, повний placement/compiler, незалежний hot reload та
   production load/storm acceptance. Preissued PKI достатньо для стенда;
   R33 management invitations не дорівнював би реєстрації Agent/випуску cert.
4. Не вигадувати aggregation typed values/metrics кількох replicas:
   непогоджені випадки відхиляються; single-replica extraction працює.

Результат перевірки поточного запуску та точні команди: див.
[acceptance-2026-11-17](../experiments/acceptance-2026-11-17/README.md).
Готовність усієї магістерської роботи або повного продукту тут не заявляється.
