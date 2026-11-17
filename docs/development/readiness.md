# Стан завершення дослідницького прототипу

Стан на 17.11.2026 (Asia/Jerusalem). Поточний програмний мінімум із
[узгодженої матриці](../planning/minimal-research-acceptance-2026-11-17.md)
реалізовано та підтверджено наведеними нижче сценаріями. Історичні оцінки
65–75% стосувалися ширшого продуктового плану; вони більше не є оцінкою
цього обсягу. Відсоток без визначеного знаменника не використовується.

| Частина | Реалізований результат |
|---|---|
| Моніторинг | Declarative registry/refs/validate/plan/apply → кілька Monitor → mTLS Agent → Blackbox/native extraction → PostgreSQL Observation |
| Оцінка | H/Q/required, revision/TTL/maxSkew, coverage, zones, same-zone all/any/quorum, CEL guards |
| Діагностика | Static/Fixed/Adaptive, pair gain/cost, eligibility, expiry rebuild, confirmation, durable incident budget, start/cancel/retry |
| Пояснення | Persisted decision journal, session revision, physical task → exact Observation; evidence links з Assessment та Alert |
| Сповіщення | Alert FSM, acknowledgement, durable webhook/retry/test, restart recovery |
| Робота оператора | REST/CLI/React, users/PAT/cookie/CSRF/RBAC, resource management, groups/dependency graph, history |
| Дослідження | Fault controls, CSV/JSON/HTML, backup/package; повний diagnostic evidence export у CLI та React з integer precision і явними missing IDs |

Підтверджений [базовий запуск і 15 fault trials](../experiments/acceptance-2026-11-17/README.md):
15 очікуваних states/results, 12 підтриманих класів та 3 multiple inconclusive.
[Фінальний export/browser run](../experiments/diagnostic-export-2026-11-17/README.md)
підтвердив decision journal, confirmation, production CLI export, React evidence
Blob, два Monitor, CRUD, session restore та logout без console exceptions.

## Що лишається поза завершеним програмним мінімумом

Повна відповідність старому v1alpha1: enrollment/certificate lifecycle,
installer, HA, independent hot reload, повний compiler/placement і production
storm/load acceptance. Зони стенда логічно розділені на одному хості;
N=3 physical replicas та фактичний OS suspend не перевірялися цим release.
Multi-replica aggregation typed values/metrics не вигадується: непогоджені
випадки відхиляються. Це явні межі поставленого дослідницького профілю.

## Залишок магістерської записки

Перенести результати й межі до роботи, заповнити §2.4–2.6 та експериментальну
частину, виправити пояснення H/Q і лічильник таблиці 2.2. Завантажений DOCX
не змінено. Приймання програмного мінімуму не підтверджує завершення тексту
всієї роботи чи її прийняття кафедрою.
