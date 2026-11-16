# ADR 0013: planned-N replica outcome і фізичне coverage

Дата: 16.11.2026 (Europe/Kiev). Реалізація погоджених Q03: all/any/quorum,
строга більшість від planned N, required failure invariant та deferred values.

Оцінюємо latest Agent/check окремо; raw observations не згортаються в storage.
N визначається плановими slots у зоні. all потребує N successes або ≥1 failure;
any — ≥1 success або N failures; quorum — floor(N/2)+1 однорідних votes. Інакше
unknown. Missing/expiry/execution failure не стають негативним голосом і не
змінюють знаменник. Duplicate replay не збільшує вагу фізичного джерела.

Звичайний контекст — latest поточної revision/TTL у maxSkew-вікні від newest
зонального timestamp. Required negative proof може бути незалежним від інших
checks, але його власний мінімальний набір witnesses має бути coherent. Після
новішої execution failure не повертаємо попередній фізичний success/failure.

Coverage повного набору фізичних результатів і Q/H логічних outcomes — різні
величини. Required missing блокує healthy; required negative ніколи не
компенсується CEL або вагами. Divergence reason та всі відповідні IDs доступні.
CEL отримує typed CheckView агрегату, а не несправжній фізичний Observation.
Quorum tie не означає execution failure. Values/metrics не агрегуються;
checked AST validation відхиляє їх використання за possible same-zone replicas.

DistinctBy=zone тепер є explicit constraint у resolved IR. Інакше roster і
selection можуть містити різні Agent однієї зони. Динамічний відбір пропускає
вже використані зони, а не відхиляє перші два кандидати за наявності третього
допустимого. Partial залишається вимкненим.

Default all і відсутні нові IR fields не змінюють старий hash.
Explicit distinctBy=zone додається до IR і створює нову revision. IR із новими полями/replica placement потребує assessment:replicas.
Наявний REST DTO/routes не змінені. DSL §9.7 та ТЗ §4.5 (Markdown і DOCX)
синхронізовані з раніше погодженими planned-N правилами. Same-zone labels не
вважаються підтвердженими незалежними failure domains.
