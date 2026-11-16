# Фізичні replicas та оцінювання в межах зони

Roster Controller може містити кілька різних довірених Agent в одній зоні:
`-slots branch=office,dc=office`. Кожний Agent виконує окремий фізичний probe,
з власним observation ID, lease/session generation і spool. Повторна доставка
одного результату не створює додаткового голосу. У roster дозволено до 100
унікальних identities; bootstrap requiredZones мають бути покриті рівно цим
набором зон. Довільне самостійне оголошення зони Agent не підтримується.

У placement `distinctBy: none` дозволяє кілька Agent з однієї зони; явний
`distinctBy: zone` забороняє такий вибір. fixed/leastLoaded/roundRobin шукають
повний feasible набір; за недостатності різних зон лишаються pending. Параметр
replicas визначає число різних Agent, а не повтори одного Agent. Per-check
placement overrides та distinctBy=failureDomain ще не підключені.

```yaml
assessment:
  requiredZones: [office]
  sameZoneAggregation: quorum
  freshness: {maxAge: 30s, maxSkew: 15s}
  checks:
    - {id: http, weight: 1, required: true}
```

Для кожного check/zone N береться з запланованих slots. У coherent вікні
рахуються тільки latest поточної effective revision, з чинним TTL, completed
execution і success/failure outcome:

| Політика | Success | Failure | Інакше |
|---|---|---|---|
| all (default) | N successes | ≥1 failure | unknown |
| any | ≥1 success | N failures | unknown |
| quorum | ≥ floor(N/2)+1 successes | ≥ floor(N/2)+1 failures | unknown |

N=0 завжди unknown. Missing, execution failure, expiry та replay не зменшують
N. Для N=2 один success та один failure — quorum tie/unknown; для N=3 два
узгоджені голоси достатні для логічного висновку.

## Coverage, час і required

Звичайний контекст містить latest кожного Agent/check у вікні maxSkew від
найновішого придатного за TTL timestamp зони. Старі результати не підміняють
новішого execution failure. Підтверджена required failure має окремий пріоритет:
за її політикою треба достатньо власних негативних witnesses у maxSkew, але
чекати інші checks не потрібно. Одного failure достатньо для all, але не для
quorum N=3 або any N=2.

Неповні required replicas блокують healthy. Фізичне coverage показує повноту
придатних результатів усіх планових джерел; Q/H — валідність і успішність
логічних check outcomes. Повне фізичне coverage при quorum tie може поєднуватися
з Q=0, H=null та unknown: усі відповіді отримано, але більшість не визначена.
H=1 за quorum означає успішний логічний check, а не успіх кожної replicas.
За протилежних поточних outcomes reason — AGENT_RESULTS_DIVERGE; raw IDs і
результати залишаються доступними. CEL не скасовує підтверджену required failure.

## CEL та межі

Outcome дозволено використовувати в CEL. Для агрегованого executionStatus:
completed — усі актуальні планові процедури завершені; failed — є execution
failure; not_executed — бракує процедури. Quorum tie не означає execution
failure. AgeSeconds — вік найстаршого observation поточного check-вікна.

Користувач відклав агрегацію values/metrics для кількох same-zone replicas.
Такі CEL rules відхиляються до збереження. Перевіряється checked CEL AST,
включно з aliases/macros; check ID або target label з назвою metrics не
забороняються. За dynamic placement враховуються можливі same-zone candidates:
використайте distinctBy: zone або окремий Monitor, якщо потрібні singleton
values/metrics. Сирі extracted values та HTTP status кожного probe зберігаються.

Same-zone replicas не доводять незалежності failure domains. Ніяких усереднень
метрик, нових DSL fields чи REST routes цей етап не додає. Controller/Agent
необхідно оновлювати разом; новий IR використовує capability assessment:replicas.

## Відтворення

`scripts/lab_replicas.py --project NEW_PROJECT --prefix 172.19 --output NEW.json`
підготує окремий приватний стенд, призначить branch/dc одній зоні, виконає
фізичні probes, створить мережевий збій лише одного шляху та перевірить all,
any, quorum tie, missing vote, required failure і recovery. Пароль тестового
admin лише в пам'яті/stdin; TLS verification увімкнено. Host ports не відкриваються.
При завершенні стенд зупиняється зі збереженням named volumes. Користуйтесь новим
project/output: наявні identities й файли не перезаписуються. Це API/Compose
сценарій двох Agent, не browser test і не трьох-Agent benchmark.
