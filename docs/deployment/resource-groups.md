# Групи ресурсів (R26)

ResourceGroup підключено до YAML/JSON registry, REST CRUD/history/status,
CLI, mixed apply та React. Це похідний погляд на MonitorAssessment: власних
probes, копій Observations чи додаткового сервісу група не створює.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: ResourceGroup
metadata: {name: business-services}
spec:
  members: [public-web, internal-api]
  mode: derived
  evaluation:
    strategy: all-required
    minCoverage: 1
```

Використовуйте members або selector, але не обидва. Members посилається тільки
на наявні Monitor; вкладених груп немає. Monitor і групу можна створити одним
mixed apply незалежно від порядку. Selector підтримує matchLabels та
matchExpressions In/NotIn/Exists/DoesNotExist на Monitor.metadata.labels.
Відсутність ключа відповідає NotIn/DoesNotExist; In/Exists потребують ключа.
Labels не перетворюють неактивний Monitor на працюючий.

Mode context (default) не має власного health: API state=null, coverageState=unknown.
Можливі порожні context members. Derived потребує evaluation; порожній склад
дає unknown. У поточному runtime група обмежена 100 Monitor; перевищення явно
відхиляється. Нових DSL-полів цей ліміт не додає.

All-required/any використовують погоджену таблицю зон, але group minCoverage
застосовується перед класифікацією. Наприклад healthy+unknown при default
minCoverage=1 дає unknown в обох режимах; явний minCoverage=0.5 дозволяє any
повернути healthy. Down+unknown при достатньому coverage дає degraded.
Відсутні MonitorAssessment не стають down.

Weighted використовує healthy=1, degraded=0.5, down=0. Unknown виключений зі
score, але зменшує зважене coverage Q. Ваги нормуються при обчисленні відношень,
не переносяться на checks. Поточний зріз потребує явних weights для кожного
fixed member; weighted selector та неповна карта ваг відхиляються. Позитивна
сума ваг обов'язкова. За відсутності healthyMin тільки score=1 дає healthy;
score=0 дає down, проміжні значення — degraded. Явний healthyMin має пріоритет.
Нормативний default у DSL §16.1 виправлений з 0.9 на 1.0 на підставі вже
погодженого користувачем Q04; інші DSL defaults не змінювались.

CoverageState відображає також неповне coverage окремого Monitor. Тому healthy
за явною any/weighted політикою може співіснувати з coverageState=degraded.
Усі учасники, їх стани та desired revisions залишаються в API/панелі. Context
не обчислює групову доступність навіть за здорових учасників.

## Актуальність і збереження

GET /api/v1/resource-groups/{name}/assessment читає membership, desired revisions
та assessments у repeatable-read snapshot PostgreSQL. Використовується тільки
збіг effective revision і snapshot не старше 5 секунд — та сама operational
межа актуальності Controller, що для наявного Assessment API. Evidence до
створення поточної identity Monitor не використовується. Старі або майбутні
snapshots стають unknown. Group metadata/spec та selector membership читаються
зі збереженого каталогу після restart.

Label change змінює membership та effective revision залежної selector group;
source manifest/resourceVersion групи не переписується. Group-only зміни не
перезапускають Agent. Plan показує implicit group reconcile. Operation reason
групи group_catalogued означає валідний запис, а не healthy; актуальна оцінка
обчислюється окремим GET. Неактивний Monitor може лишати свою operation pending.
Це не завершена реалізація multi-Monitor execution.

Explicit member Monitor не можна видалити, доки існує групове посилання (409).
Для selector видалення/зміна labels перераховує склад. CRUD зберігає scopes,
CSRF, ETag/If-Match, audit та idempotency інших ресурсів. Read endpoint потребує
resources:read. OpenAPI уточнений типізованим GroupAssessment/Member DTO без
зміни маршруту; state context допускає null.

```sh
netmonorc get --context-file /private/context.json ResourceGroup
netmonorc get --context-file /private/context.json groups business-services
netmonorc apply --context-file /private/context.json -f groups.yaml --wait
```

У React: ResourceGroup у конфігураційному редакторі та окрема панель «Групи
ресурсів» із membership/state/coverage і ручним оновленням. Бізнес-формули в
браузері не дублюються. Build не означає виконаний browser click test.

Критичні use cases: unknown/stale/revision mismatch, default і explicit weighted
threshold, context без health, зміна selector membership, PG JSONB round trip,
referenced delete/If-Match guards. Live сценарій lab_sessions.py --groups-only
працює на новому owned stand зі штатним login; фактичний результат зберігається
окремо у docs/experiments/resource-groups-r26 після завершення.
