# NetMonOrc

Мінімальний дослідницький прототип методу комплексного моніторингу.
Робочий зріз — локальний ланцюг YAML → native HTTP / Blackbox → Observation → Assessment.
Повної відповідності попередньому v1alpha1 поки немає.

**Новий розподілений зріз:** окремі Controller/Agent, gRPC/mTLS,
lease, локальна черга та PostgreSQL. Запуск і межі реалізації:
[docs/architecture/distributed-slice.md](docs/architecture/distributed-slice.md).

## Запуск першого зрізу

Потрібні Go ≥1.25 і Python 3 для демонстраційного HTTP-сервера.

```bash
go mod download
go build -o .local/netmonorc ./cmd/netmonorc
python3 scripts/smoke.py
```

Демонстрація запускає справжній HTTP-сервер на `127.0.0.1:18080`,
виконує запити через зібраний CLI та перевіряє три результати:
усі успіхи → healthy; відсутня зона → unknown;
HTTP 503 у філії та HTTP 200 у серверній зоні → degraded.
Обидва джерела тут локальні: це **не** експеримент із незалежними мережевими зонами.

Команди окремо:

```bash
.local/netmonorc validate -config examples/http.yaml
.local/netmonorc probe -config examples/http.yaml -agent branch -zone office -allow-cidrs 127.0.0.1/32 > .local/office.ndjson
.local/netmonorc assess -config examples/http.yaml -slots branch=office,dc=server < .local/office.ndjson
```

Для `probe` потрібен доступний HTTP-сервіс із прикладу. Команда `assess`
читає NDJSON зі стандартного вводу; заплановані агенти задаються окремо,
тому втрата джерела не зменшує очікуване покриття.

## Blackbox Exporter

Підтримується підготовлений executable **Blackbox Exporter 0.27.0**.
Для HTTP check використовуйте `runner: blackbox` або `runner: auto`;
відсутній runner також обирає Blackbox відповідно до DSL.
CLI запускає один керований exporter на виклик `probe` із приватним
згенерованим конфігом і випадковим loopback-портом; при завершенні
процес і тимчасовий конфіг прибираються. Довільні аргументи, модулі
або executable paths у YAML не допускаються.

TCP, DNS (UDP/TCP) та ICMP також підтримуються через exporter:
[мережеві перевірки й запуск стенда](docs/architecture/network-probes.md).

```bash
python3 scripts/smoke.py --blackbox-binary /path/to/blackbox_exporter
.local/netmonorc probe -config .local/blackbox-smoke.yaml -agent branch -zone office -allow-cidrs 127.0.0.1/32 -blackbox-binary /path/to/blackbox_exporter
```

Для Cloud binary встановлено в
`/workspace/toolchains/blackbox_exporter-0.27.0.linux-amd64/blackbox_exporter`.
Офіційний Linux amd64 archive має SHA256
`507a77ff411822fd6b543e8b8d9e00e1cc49408df465588cd746753bf05046e2`.
Джерело: GitHub release `prometheus/blackbox_exporter`, tag `v0.27.0`.

HTTP runner-и приймають лише явні IP-literal URL з дозволених CIDR;
DNS-цілі, redirects, proxy та вимкнення перевірки TLS недоступні.
Це обмежена інтеграція з IP-literal targets, а не завершена мережева політика
для всіх протоколів. Startup exporter враховується в часі CLI-команди;
продуктивність майбутнього постійного Agent цим запуском не вимірюється.

## Межі поточної реалізації

- Один Monitor з inline target, HTTP/TCP/DNS/ICMP checks та inline weighted assessment.
- `native-http` та `blackbox`/`auto`, GET/HEAD, IP-literal URL, перевірка TLS,
  обмеження body/headers/timeout, явний CIDR allowlist, без redirects.
- Required checks, H/Q, TTL для кожної перевірки, maxSkew,
  поточна ревізія, newest observation, фіксований агент на зону,
  зональна агрегація all-required.
- Непідтримувані YAML-поля та можливості відхиляються.
- Це локальні CLI-операції, без нового альтернативного HTTP API.

Розподілений запуск підтримує фіксовані джерела, mTLS, послідовне регулярне
виконання й PostgreSQL. Наразі немає динамічного Placement/Scheduler,
CEL/extraction, діагностики, алертів і вебпанелі.
CLI не автентифікує агентів: NDJSON придатний для контрольованого стенда,
а не для приймання недовірених мережевих результатів. Мережеві результати
приймає окремий gRPC Controller з перевіркою mTLS principal та assignment.

## Розробка

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
```

HTTP-тести й smoke потребують дозволу на локальні сокети.
У поточному Cloud компілятор встановлено в `/workspace/toolchains/go/bin`;
використовуйте цей PATH і writable кеш:

```bash
export PATH=/workspace/toolchains/go/bin:$PATH
export GOCACHE=/workspace/.cache/netmonorc-go
```

План і фактичний стан: [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).
