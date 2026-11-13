# NetMonOrc

Мінімальний дослідницький прототип методу комплексного моніторингу.
Перший робочий зріз — локальний ланцюг YAML → HTTP probe → Observation → Assessment.
Повної відповідності попередньому v1alpha1 поки немає.

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

## Межі поточної реалізації

- Один Monitor з inline target, HTTP checks та inline weighted assessment.
- Явний `native-http`, GET/HEAD, IP-literal URL, перевірка TLS,
  обмеження body/headers/timeout, явний CIDR allowlist, без redirects.
- Required checks, H/Q, TTL для кожної перевірки, maxSkew,
  поточна ревізія, newest observation, фіксований агент на зону,
  зональна агрегація all-required.
- Непідтримувані YAML-поля та можливості відхиляються.
- Це локальні CLI-операції, без нового альтернативного HTTP API.

Наразі немає мережевого Controller/Agent, mTLS, Scheduler, Blackbox,
PostgreSQL, CEL/extraction, діагностики, алертів і вебпанелі.
CLI не автентифікує агентів: NDJSON придатний для контрольованого стенда,
а не для приймання недовірених мережевих результатів.

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
