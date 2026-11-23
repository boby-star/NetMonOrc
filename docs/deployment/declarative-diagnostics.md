# DiagnosticBinding, Fixed evidence та спільний placement

Закриває три конкретні напрями [аудиту YAML/CEL](../specs/yaml-cel-audit-2026-11-22.md):
ранню перевірку тригера, values/reasonCode у Fixed та placementRef/selectors
діагностичних дій. Не є реалізацією всього розширеного CEL context §18.

## DiagnosticBinding.when

Monitor context став закритим native CEL object: name, state, coverageState —
string. `monitor.sttae` більше не проходить validate. Контекст і coverage
обчислюються з реального Assessment за попередніми правилами, збережено
cost=1024, evaluation timeout=25ms та довжину expression ≤2048 bytes.
Labels/weightedScore/target цим етапом не додані: вони явно відхиляються.
Звичайні звернення до name/state/coverageState зберігають семантику й source
hash. Вирази, специфічні для довільної map замість типізованого monitor object,
можуть бути відхилені раніше; джерело конфігурації автоматично не переписується.

## FixedStep.when: докази попередніх tasks

TaskView має outcome, executionStatus, values та reasonCode. Values містить
лише типізовані scalar з фактичного Observation: integer/int64, float/double,
boolean або string. Передавання через protobuf wrappers у CEL зберігає
9007199254740993, integer zero та false без округлення або підстановок.
Відсутній capture лишається відсутнім map key. Для перевірки наявності:

```cel
has(tasks["a-read"].values.depth) &&
tasks["a-read"].values.depth == 0 &&
tasks["a-read"].values.ready &&
tasks["a-read"].reasonCode == "expected_status"
```

Reason code є фактичним кодом probe, без вигаданого перекодування у приклади
DSL. У native HTTP success це expected_status. Transport передає дані planner
лише після durable CommitBatch і перевірки task/Agent/revision provenance.
Engine використовує values лише для завершеного, коректного результату з
Observation ID і часом. Помилка runner не створює values цілі. Після ValidUntil
values прибираються з guard context. Нові дані не додаються у Decision DTO чи
profile hash: provenance й raw values лишаються в linked Observation.

Serial і parallel engines передають окремий context лише прямих/транзитивних
попередників. Retry використовує terminal attempt activation, без даних
незалежних гілок. Runtime error через відсутнє значення блокує крок і лишає
inconclusive; false — звичайний skip. Бюджет додаткового probe витрачається
тільки при фізичному запуску. Немає нової можливості виконувати код/мережу з CEL.

## DiagnosticAction placement

Підтримано placementRef з oneOf inline placement, selectors matchLabels/
matchExpressions, agentRefs/zones, fixed/leastLoaded/roundRobin та distinctBy.
PlacementRef розгортається через той самий PlacementPolicy compiler; inline
також перевіряється ним. Selector normalization спільна для regular і diagnostic
шляхів. Ref і hash політики входять до compiled profile hash; source не
переписується. Каталог компілює політики до залежних DiagnosticProfile.
Відсутній ref, неправильний selector, одночасні ref/inline відхиляються.

Runtime перетинає selector з refs/zones, capabilities, online/capacity та
Controller-owned eligibility/target policy. Labels беруться із Server.AgentLabels,
а не Agent Hello. Нуль відповідних Agent не означає fallback на довільного.
Для distinctBy=failureDomain потрібен відомий Controller failureDomain.
Ручний запуск завантажує лише зовнішні refs профілю; transactional admission
звіряє його hash з актуальним effective revision, тому rollout не обходить fences.

Збережено replicas=1 для action та onInsufficientAgents=pending. Partial і
replicas>1 явно відхиляються; для кількох точок задавайте окремі actions з
окремим бюджетом. При одній replica zone/domain distinctness не доводить
незалежності кількох фізичних джерел. Default inheritance від Monitor не
додано: розміщення дії слід задавати явно для дослідження зон.

[Повний YAML-приклад](../../examples/diagnostics/parsed-fixed.yaml) містить
PlacementPolicy та DiagnosticProfile: прочитати body в office, розпарсити
queue_depth/ready, за значеннями й reason запустити confirm у server.
[Відтворення та результати](../experiments/declarative-diagnostics-2026-11-23/README.md).

Решта залишку: повний monitor/target/checks/diagnostic context Fixed, monitor
labels/score у trigger, multi-replica diagnostic execution та Alert zone scope
не закриваються цією ітерацією. Нових HTTP routes чи browser-side обчислень немає.
