# Розподілений дослідницький зріз

## Реалізований ланцюг

`Monitor YAML → Controller → gRPC/mTLS → Agent → runner → durable spool → PostgreSQL → ACK → Assessment`.

Controller і Agent — окремі Go-процеси. Controller залишається модульним
монолітом. Ресурсні моделі та математичне ядро не залежать від gRPC і pgx.
Нових публічних REST-маршрутів немає; локальний JSON-файл оцінки є
діагностичним артефактом, а не альтернативою OpenAPI.

## Довіра й протокол

- TLS 1.3, взаємна перевірка сертифікатів, без insecure режиму.
- Agent identity — єдиний URI SAN `spiffe://netmonorc/agent/<id>`.
  CN та IP не використовуються як автентифікація.
- Адміністратор задає Controller фіксований roster `agent=zone`, по одному
  Agent на потрібну зону. Agent не призначає собі зону.
- Сертифікати для цього мінімального профілю підготовлені наперед.
  Enrollment, автоматична ротація та API відкликання ще не реалізовані.
- `.proto` містить початкову підмножину ТЗ §9: Connect, Hello,
  ConfigAssignment/Applied, ObservationBatch/Ack. Додано Heartbeat/load
  та DiagnosticTask/TaskReceipt, best-effort TaskCancel (R13); результати задач ідуть ObservationBatch.
  CertificateRenewal і решта повного agent protocol відсутні.
- Підтримується одна версія `v1alpha1`; IR передається як JSON у bytes
  типізованого Protobuf envelope з checksum і effective revision.
- Максимум frame — 1 MiB, batch — 32 спостереження, план — 100 checks.

## Lease і відновлення

Controller поновлює 30-секундний lease кожні 10 секунд. Agent перевіряє
checksum та canonical revision, bounds плану й commit локального assignment
до ConfigApplied. Час виконання обмежений монотонним deadline.

Для стенда потрібні синхронізовані годинники з похибкою не більше 5 секунд.
Agent відхиляє Controller frames поза цим часовим вікном і віднімає
5 секунд від залишку lease, щоб не подовжувати його за такої похибки.
Це консервативна межа довіри, а не вимірювання clock offset чи заміна maxSkew.
Часовий збій може зменшити coverage; він не доводить відмови сервісу.

Під час disconnect регулярні checks тривають тільки до чинного lease.
Після завершення lease нові checks не стартують, незавершені обмежуються
deadline. Після перезапуску процесу Agent збережений assignment не
відновлюється для виконання: потрібен новий підтверджений lease.
Це консервативний fail-closed профіль без оптимізації відновлення same-boot.

## Доставка і сховище

Черга Agent — приватний каталог із process lock та максимум 1000 записів.
Кожне спостереження має сталий ID. Запис, rename і запис каталогу
синхронізуються на диску перед передаванням. За заповнення черги
нові probes призупиняються; наявні записи не видаляються для звільнення місця.

Controller зберігає спостереження транзакцією PostgreSQL, після чого
надсилає accepted/duplicate receipts. Agent видаляє лише записи з
підтвердженого in-flight batch. Конфлікт того самого ID з іншим payload
відхиляється; втрата ACK призводить до безпечного повтору доставки.
Це не exactly-once фізичне виконання probe.

PostgreSQL також зберігає session generation, історичні bindings
`agent/revision/zone/monitor` та поточний Assessment. Нова сесія
збільшує generation. Перевірка generation утримує row lock до commit,
тому старий stream не може записати результати після його заміни.
Історичні bindings перевіряють походження replay навіть після зміни
зони або ревізії. Старі результати зберігаються, але поточний Assessment
обчислюється лише для чинної ревізії та запланованих джерел.

Controller обчислює стан щосекунди, зокрема без надходження нових даних,
щоб TTL справді міг перевести попередній healthy в unknown.
DDL створює початкову схему `nmo_*`; оновлення старих схем і retention
ще не є реалізованою системою міграцій/обслуговування.

## Перевірка

Потрібні Go ≥1.25, Python 3, OpenSSL, Docker з доступним daemon.

```bash
go build -o .local/netmonorc-controller ./cmd/netmonorc-controller
go build -o .local/netmonorc-agent ./cmd/netmonorc-agent
python3 scripts/distributed_smoke.py
```

Сценарій використовує PostgreSQL 17.6 Alpine з digest,
створює унікальний контейнер і тестові PKI-файли у `.local/`, запускає
Controller та два Agent-процеси й перевіряє:

1. Без спостережень — unknown; із двома автентифікованими джерелами — healthy.
2. Чужий CA не підтверджує Controller.
3. Disconnect спричиняє буферизацію, restart не губить чергу.
4. Повтор уже committed observation не створює другого запису.
5. HTTP 503 — down, відновлення HTTP 200 — healthy.
6. Втрата Agent і завершення TTL — unknown, а не down.
7. SQL-тест перевіряє конфлікт ID і generation fencing.

Сценарій видаляє лише власний контейнер та його тестові томи;
логи й snapshot залишаються у приватному `.local/distributed-*`.
Ці каталоги містять тестові секрети — вони не входять до Git.
PostgreSQL у цьому локальному сценарії публікується лише на loopback
випадкового host-порту; TLS до БД вимкнено лише для цього стенда.

Обидва джерела працюють на одному хості. Це перевірка розподілених
процесів і доставки, не доказ незалежності failure domains і не
порівняльний мережевий експеримент.

## Самостійний запуск

Підготуйте власний приватний PostgreSQL і DSN-файл із правами 0600.
Не передавайте пароль як CLI-аргумент. Для локального PKI:

```bash
python3 scripts/dev_pki.py .local/pki
.local/netmonorc-controller -config examples/http.yaml -database-file .local/postgres.dsn -slots branch=office,dc=server -ca .local/pki/ca.crt -cert .local/pki/controller.crt -key .local/pki/controller.key
.local/netmonorc-agent -controller 127.0.0.1:9443 -data-dir .local/branch -allow-cidrs 127.0.0.1/32 -ca .local/pki/ca.crt -cert .local/pki/branch.crt -key .local/pki/branch.key
```

Другий Agent запускається з `dc.crt`, `dc.key` і окремим каталогом.
HTTP-ціль із маніфесту має бути доступною. `--help` показує всі flags.
`blackbox` працює тим самим driver-ом, що локальний CLI; шлях задається
параметром `-blackbox-binary`. Distributed smoke підтримує native-http,
мережеві checks через `--network` і діагностичні probes через
`--diagnostics adaptive --diagnostic-runner blackbox`.

## Генерація контракту

Generated Go-код збережено в репозиторії, для звичайного build protoc не потрібен.
Використано grpcio-tools 1.76.0 (protoc 6.31.1), protoc-gen-go 1.36.10,
protoc-gen-go-grpc 1.5.1:

```bash
python -m grpc_tools.protoc -I . --go_out=. --go_opt=module=github.com/boby-star/NetMonOrc --go-grpc_out=. --go-grpc_opt=module=github.com/boby-star/NetMonOrc proto/netmonorc/agent/v1/agent.proto
```

Go plugins мають бути в PATH. Погоджений confirmation контракт доповнює
DSL §11.6.1 та додаток ТЗ/DOCX; публічний HTTP OpenAPI не змінюється.
