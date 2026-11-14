# CLI для конфігурацій та операцій (R8)

Після `netmonorc login` CLI використовує збережений приватний PAT-контекст.
Додані remote validate, plan, apply, apply --wait і get Monitor/operations.
Вони працюють з тими самими HTTP endpoints, DTO та серверною валідацією, що
React. Паролі/токени не передаються аргументами; TLS перевіряється завжди.

```sh
netmonorc validate --remote -f monitor.yaml
netmonorc plan -f batch.json
netmonorc apply -f batch.json --wait --timeout 2m
netmonorc get Monitor research-web
netmonorc get operations OPERATION_ID
netmonorc get --state running --limit 20 operations
```

Підтримуються JSON/YAML ConfigBatch та один або кілька YAML Monitor documents
через `---`. `-f -` читає stdin. Максимум — 1 MiB вхідного файла/запиту та
20 manifests. JSON-подання spec передається Controller; клієнт не повторює
DSL defaults, CEL або правила оцінювання. Невалідна remote validation друкує
ValidationResult і завершується ненульовим exit code.

Для заміни потрібний expectedVersions із GET ресурсу. Вхідний ConfigBatch
вже може містити цей map і requestId; для YAML manifests є окремі flags:

```sh
netmonorc apply -f monitor.yaml --expected-versions versions.json --request-id update-network-001 --wait
```

versions.json — JSON object виду `{"Monitor/research-web":"RESOURCE_VERSION"}`;
для нового Monitor використовуйте null або відсутню precondition. Версію
беріть із resourceVersion, без ETag quotes. Apply наявного ресурсу без неї
відхиляється сервером, зміни не застосовуються. CLI не підмінює ці preconditions
автоматичним читанням latest під час повтору.

CLI apply вимагає явний стабільний requestId у batch або --request-id.
Збережіть незмінні файл/flags для повтору після втрати відповіді. Інший requestId
для іншого пакета; різні ключі у файлі та flag відхиляються локально.
Idempotency на сервері діє 24h у межах user ID. Ті самі input/flags дають
однакове нормалізоване тіло; JSON/YAML клієнт нормалізує під час читання.

--wait очікує саме completed операції за ACK, не healthy ресурсу. ID друкується
в stderr одразу після приймання. Timeout (1s..30m, default 1m) або недоступність
клієнта не відкочує прийнятий desired. Відновіть читання через get operations.
Операція failed друкується як JSON і команда завершується з помилкою. Під час
очікування повторюються тільки GET за transient I/O/429/503; POST не повторюється
автоматично. Відповідь чужого operation ID не приймається як завершення очікуваного.

Read commands потребують resources:read, plan/apply/remote validate — admin
і resources:write; apply --wait потребує обох scopes. Є --context-file для
окремого приватного контексту. З --limit/--cursor можна переглядати сторінки;
state застосовується лише до operations. Ця stdlib flags-версія вимагає flags
перед positional arguments у get. Output поки JSON; Cobra/named contexts,
інші kind/команди та table/yaml output залишаються наступними етапами.

Без --remote існуючий validate -config/-f лишається локальною дослідницькою
перевіркою. Probe та assess також лишаються локальними операціями.

HTTP client має TLS≥1.3, trusted CA, заборону redirect, bounded request/response
і таймаути. Текст HTTP error bodies не друкується; лише status і safe error code.
JSON read output зберігає цілі числа без float64-перетворення. Один критичний
Compose-сценарій перевіряє YAML validation → CLI plan/apply --wait → фізичні
ACK → get completed після restart, та відновлення початкового manifest.
