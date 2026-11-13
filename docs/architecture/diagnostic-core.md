# Ядро діагностики та відтворюваний replay

## Фактичний результат

`internal/diagnostics` виконує три режими через один Runner і один набір
обмежень. Воно не залежить від PostgreSQL, gRPC або exporter.
`CompileDiagnostics` читає закриту підмножину нормативного DiagnosticProfile.
Це завершений алгоритмічний компонент, але поки **не автоматична мережева
діагностика Controller**: DiagnosticTask/TaskResult та durable coordinator
ще не підключені. Публічного start API немає.

- Adaptive: кількість пар сумісних гіпотез із неперетинними очікуваними
  результатами / costUnits. Формула відповідає ТЗ §4.9.
- `either` допускає success/failure/unknown, тому не створює штучного gain.
- Дії за однакового Score впорядковуються за ID; виконавець обирається
  за fixed/leastLoaded/roundRobin після фільтрів і обов'язкового policy gate.
  Невідома місткість не вгадується: Capacity<=0 не допускається до виконання.
- Fixed: DAG із перевіркою циклів, dependsOn та CEL when. Контекст містить
  tasks і monitor.state; результат повинен бути bool. CEL має cost limit.
- Static — лише режим дослідницького runner-а. Нового `mode: static`
  у YAML не додано.
- Всі режими зараз послідовні, отже не перевищують maxParallel.
  Переваг за latency для паралельних baseline ця реалізація не доводить.

## Коректність і межі

Технічна помилка не звужує гіпотези. `completed` вимагає одного
сумісного класу й конкретного success/failure evidence для його pattern.
Unknown/either самі по собі не підтверджують клас. Fixed без hypotheses
або без достатніх expectedOutcomes завершується inconclusive.

Чиста функція MatchBaseline зіставляє лише еквівалентні check/runner/config,
включно з timeout. Вона перевіряє revision, TTL, агента/зону, latest result
та узгоджене часове вікно. Новіший execution failure не відновлює старий
успіх. Вхідні observations мають походити з автентифікованого ingestion.
Цей helper також ще не викликається автоматичним Coordinator.

Ledger утримує час і використані спроби епізоду та забороняє одночасний
Run. Повторний Run з тим самим Ledger не скидає ліміт і не повторює
Adaptive actions. Budget policy не можна замінити для збільшення залишку.
У Fixed явне повторне actionRef — окремий облік спроби.
Ledger зараз у пам'яті: його не можна видавати за durable episode state.

Без допустимих дій, за no-gain, конфлікту доказів чи завершення бюджету
повертається inconclusive з причиною. estimatedBytes не обчислюються
з costUnits: за такого обмеження потрібен окремий відомий estimator;
за його відсутності кандидати не запускаються.

Поки підтримуються replicas=1, stopWhenUnique=true та maxNoGainSteps=0.
Confirmation/retry, stopWhenUnique=false, selectors і placementRef
відхиляються як непідтримувані. Повторне підтвердження за часом і
failure domains ще не реалізовано. Нормативні ТЗ/DSL не змінені.
Надзвичайні costUnits, які переповнюють finite float score/report,
відхиляються явно, а не породжують Inf у JSON.

## Приклад порівняння

```bash
go build -o .local/netmonorc-experiment ./cmd/netmonorc-experiment
.local/netmonorc-experiment
```

Команда читає `examples/diagnostics/{adaptive,fixed}.yaml` та
`scenarios.json`, перевіряє однаковість action catalog, hypotheses і budgets,
виконує 15 runs та зберігає JSON/CSV у `.local/diagnostics-replay.*`.
Ground truth використовується лише для звіту; Engine його не отримує.

**Дані синтетичні. Мережеві probes не запускаються.** Attempts у цьому
звіті — кількість викликів replay runner, а не виміряні IP-пакети.
Не вимірюються мережевий трафік, p95 latency або продуктивність мережі.
Це відтворюваний приклад алгоритму перед натурними експериментами.

| Синтетична несправність | Static, кроків | Fixed, кроків | Adaptive, кроків |
|---|---:|---:|---:|
| DNS | 4 | 1 | 1 |
| Застосунок | 4 | 3 | 2 |
| Доступ із філії | 4 | 3 | 2 |

Fixed використовує звичний порядок DNS → TCP → HTTP із умовами,
а не навмисно виконує всі перевірки завжди. За технічної неможливості
виконання Fixed припиняє гілку після DNS і витрачає менше кроків,
ніж Adaptive у цьому прикладі.

У наборі також збережено **контрприклад** outside-catalog-all-success:
рання зупинка Adaptive повертає zone-path-failure, хоча truth — поза
каталогом. Static та Fixed отримують додаткову суперечність і утримуються
від класифікації. Це фактичне обмеження closed-catalog/early-stop моделі,
яке не приховано зі звіту. Інтеграція підтверджувальних перевірок та
розширення каталогу потрібні перед заявами про якість діагностування
нестабільних чи невідомих несправностей.

Replay не є способом ручного запуску діагностики для Monitor і не
обходить diagnostics.enabled=false: він не взаємодіє з Controller/Agent.
