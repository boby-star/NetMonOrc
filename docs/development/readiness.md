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
| Діагностика | Static/Fixed/Adaptive, pair gain/cost, eligibility, expiry rebuild, confirmation, durable incident budget, start/cancel/retry, спільне admission місткості Agent до списання бюджету |
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

## Координація місткості за §2.3

Після додаткового аудиту закрито contention між діагностиками різних Monitor:
[shared admission](../architecture/diagnostic-admission.md). Натурний сценарій
підтвердив неперекривні probes та продовження regular observations.
Глобальна FIFO fairness/HA не є заявленими можливостями цієї реалізації.

## Відомі домени відмови (§2.1 та §2.3)

Controller приймає явне trusted agent=domain призначення через -failure-domains.
Невідомі домени не зараховуються як незалежні; зона не є fallback. Decisions
та confirmation зберігають історичні домени; baseline перед новим topology
mapping не переінтерпретується. [Контракт і сумісність](../deployment/failure-domains.md).
Це не додає автоматичного визначення топології або placement distinctBy=failureDomain.

## Часова придатність діагностичних доказів (§2.1–2.3)

Після додаткової звірки закрито накопичення нових diagnostic proofs без
спільного TTL/maxSkew: [часове вікно](../architecture/diagnostic-evidence-window.md).
Планувальник і confirmation використовують фізичний час записаного Observation,
latest-pair semantics і зберігають budget після відкидання доказів. Нові report,
API та React показують policy/timing; legacy timestamps не реконструюються.
[Три натурні сценарії](../experiments/temporal-evidence-2026-11-18/README.md).

## Початкові докази діагностики (§2.1–2.3)

Збережено provenance регулярного baseline, який може звузити гіпотези без
додаткових запусків. API/React показують історичний вхід, CLI/React включають
його Observation до пакета з окремою перевіркою regular provenance.
Legacy без збереженого набору позначається невідомим; baselineEvidenceKnown
не підміняється поточним Assessment. [Натурний zero-task сценарій і CLI export](../experiments/baseline-evidence-2026-11-18/README.md).

## YAML controls та детермінованість

Додано enabled, priority/jitter, inline target, unweighted/defaultState,
maxNoGainSteps; канонізовано diagnostic hash, посилено YAML/JSON validation,
offline validate використовує registry. [Контракт із явним залишком](../deployment/yaml-controls.md).
[Натурні сценарії та compiler audit](../experiments/yaml-controls-2026-11-18/README.md).
Це не завершення per-check placement, fallback, selectors, Component та інших
полів повного старого DSL.

## Trusted metadata та per-check placement

Закрито check-level placement/ref, селектори Agent labels і distinctBy=failureDomain.
[Контракт](../deployment/check-placement.md) та [натурні докази](../experiments/check-placement-2026-11-19/README.md).
Окремі checks мають власні sources/coverage; БД зберігає ownership для старого
replay. Решта fallback/Component/HTTP/parallel/byte budget ще не оголошена готовою.

## HTTP YAML policy

Додано connectIP/headers/followRedirects, native Agent DNS/IP guard та capability
http:policy-v2. [Обмеження драйверів](../deployment/http-policy.md) збережено явно;
verifyTLS:false не дозволено. [Натурний сценарій](../experiments/http-policy-2026-11-19/README.md)
підтвердив успішний pinned redirect та блокування metadata IP як unknown.

## Live estimated-byte budget

maxEstimatedBytes тепер має Coordinator gate і durable atomic PostgreSQL
reservation для кожного task/retry. API/React показують оцінку окремо від
costUnits. [Модель](../deployment/diagnostic-byte-budget.md) не є виміряним
wire traffic. [Натурне приймання](../experiments/byte-budget-2026-11-19/README.md)
підтвердило zero/exact limit, restart і no-refund/concurrency у справжній БД.
