# Same-zone fallback → diagnostic baseline → export

Джерело вимог: актуальна [редакція записки](../../specs/thesis-update-2026-11-22.md),
§2.1–2.4; DSL ExecutionFallback та DiagnosticProfile. [Правила реалізації](../../architecture/fallback-baseline.md).

Owned project `nmo-fallback-proof2-20261122`: production Controller/Agent/CLI,
PostgreSQL, HTTPS API, mTLS Agent transport і справжній HTTP probe через Blackbox.
Обидва Agent належать office, primary=branch, spare=dc. Новий DiagnosticProfile
створено через API, Monitor оновлено з If-Match; використано точну effective
revision. Автоматичний trigger вимкнений виразом `false`, сесію запущено вручну.

`live.json` підтверджує:

- до відмови primary отримано healthy на поточній revision;
- primary зупинено; durable root має причину agent_unreachable, одну витрачену
  fallback-спробу та справжній Observation від dc; первинний Observation для
  цього пропущеного запуску не вигадано;
- отриманий fallback увійшов у поточний Assessment, потім у початкові докази
  діагностики; session=completed, remainingHypotheses=[responding],
  additionalChecks=0, tasks=[];
- збережений baseline має фактичні Agent/zone, origin=branch і точні run/task
  links; REST Observation повернув ті самі атрибути зі сховища;
- production CLI через scoped PAT експортував complete=true пакет із відомим
  baseline, одним Observation та нульовим числом діагностичних tasks;
- primary відновлено, Assessment повернувся до healthy на тій самій revision.

Unit cases перевіряють відкидання незбагачених/підмінених fallback, неправильних
revision/zone/task, збереження фізичного failureDomain, детермінований вибір
при однаковому часі й різному порядку даних та відмову експорту при підміні
run/origin або відсутності metadata. `go.log`, `vet.log`, `ui-build.log`,
`build.log`: повний Go suite, vet, production React і Docker build. DSN-залежні
unit tests за звичайного запуску можуть бути пропущені; наведений live trial
працював із реальною PostgreSQL. `openapi-validation.json` перевіряє YAML і
локальні $ref, не повну відповідність HTTP контракту.

Відтворення з кореня репозиторію (потрібні Docker, зібрані образи й вільні
приватні мережі; helper відмовляється перезаписувати наявний owned project):

```sh
python scripts/lab_fallback_baseline.py prepare --project nmo-fallback-proof-new --prefix 172.27
python scripts/lab_fallback_baseline.py verify --project nmo-fallback-proof-new --prefix 172.27 --output .local/fallback-proof-new/live.json
python scripts/lab_fallback_baseline.py down --project nmo-fallback-proof-new --prefix 172.27
```

Первинна спроба helper виявила відсутній обов'язковий adaptive block у fixture;
після виправлення фінальний trial виконано в окремому свіжому project. Обидва
стенди зупинено без видалення `.local`, PKI або volumes.

Межі: один Docker host; нових browser clicks немає, React лише зібрано.
Не перевірено всі комбінації причин fallback, рестарти в кожній фазі чи фізично
незалежні failure domains. Different-zone не використовується як baseline.
Нуль додаткових diagnostic tasks не скасовує вже витрачений fallback budget.
Цей результат не є заявою про 100% виконання записки або старого ТЗ.
