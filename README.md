# NetMonOrc

NetMonOrc — исследовательский прототип комплексного мониторинга: YAML →
Controller → gRPC/mTLS → Agent → Observation → Assessment → REST/React.
Реализованы несколько Monitor, декларативные ресурсы, CEL/extraction,
Static/Fixed/Adaptive диагностика, зональные alerts, webhook, users/PAT.
Полный объём старого ТЗ ещё не реализован; актуальные ограничения находятся в
[IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).

## Варианты запуска

- **Сервер и отдельные Agents:** инструкция ниже, файлы
  [server.compose.yaml](deployment/server.compose.yaml) и
  [agent.compose.yaml](deployment/agent.compose.yaml). Они не создают тестовые
  цели, лабораторные маршруты или второй Agent на мастер-узле.
- **Демонстрация на одной машине:** [quickstart](docs/deployment/quickstart.md).
  Корневой `compose.yaml` — лаборатория с PostgreSQL, двумя Agents, тестовой целью
  и webhook-приёмником, а не готовая конфигурация удалённых серверов.

Термин «мастер» ниже означает один Controller с PostgreSQL. Agents устанавливаются
в тех сетевых зонах, из которых действительно нужно проверять доступность.

## Требования для установки

| Компонент | Мастер / Controller | Каждый Agent |
|---|---|---|
| ОС | Linux, проверяемый профиль — Linux amd64 | Linux amd64; другие архитектуры отдельно не подтверждены |
| Запуск контейнеров | Docker Engine и Compose v2 с `up --wait` | Docker Engine и Compose v2 |
| Данные | PostgreSQL **17.6**, постоянный volume; отдельный каталог snapshots | Постоянный приватный каталог spool, владелец UID/GID **10001:10001**, mode **0700** |
| TLS | CA, serverAuth cert с DNS/IP SAN сервера, private key | Тот же публичный CA, собственные clientAuth cert/key с URI SAN |
| Время | Синхронизация NTP/chrony/systemd-timesyncd | Расхождение с Controller не более **5 секунд** |
| Проверки | Controller не выполняет probes вместо Agents | Доступ к целям из своей зоны; явный CIDR allowlist |
| Blackbox Exporter | Не требуется для вычисления Assessment | **0.27.0** для TCP/DNS/ICMP и `blackbox`/`auto`; включён в общий image |

Ориентир для небольшой установки, **не измеренный минимум**: мастер — 2 vCPU,
2 GiB RAM; Agent — 1 vCPU, 512 MiB RAM. Сборочной машине желательно иметь
20 GiB свободного диска; объём БД рассчитывайте по частоте и истории проверок.
Автоматическая retention-политика БД пока не реализована.

Go и Node на узлах запуска **не нужны**: Dockerfile собирает приложение и React.
В нём закреплены Go **1.25.4**, Node **24** и Blackbox **0.27.0**.
Для сборки без Docker нужны Go ≥1.25, Node 24/npm и `npm ci` в `web/`;
для работы UI Controller дополнительно нужен результат `npm run build` (`web/dist`).
`protoc` не нужен: generated Go-файлы находятся в репозитории.
Python ≥3.10 и OpenSSL нужны для приведённой подготовки файлов; Git — для checkout.
Сборочной машине дополнительно нужен **Docker Buildx/BuildKit**: Dockerfile
использует cache/secret mounts. Проверьте `docker buildx version` до сборки.
Инструкция предполагает обычный rootful Docker; rootless UID mapping и
SELinux labels для bind mounts требуют адаптации к соответствующему host.

### Сеть и порты

| Соединение | Доступ |
|---|---|
| Agent → Controller | TCP **9443**, исходящий для Agent, входящий для Controller; gRPC/HTTP2 поверх mTLS, TLS ≥1.3 |
| Браузер / CLI → Controller | HTTPS TCP **8443**; в примере опубликован только на loopback мастера, доступ через SSH tunnel |
| Controller → PostgreSQL | TCP **5432** только во внутренней Docker-сети; host port не публикуется |
| Agent → проверяемые сервисы | Порты из YAML: HTTP/HTTPS/TCP, DNS UDP/TCP к указанному resolver; ICMP при наличии прав |
| Сборка / установка | Доступ к registry образов, Go/npm dependencies; после загрузки image Agent не требует доступа к GitHub/npm |
| Controller → webhook | HTTPS к явно разрешённому IP/CIDR, если доставка включена |

Входящий порт на Agent и публичный порт Blackbox не нужны. Exporter запускается
Agent как дочерний процесс на loopback. Root/NET_ADMIN для обычных HTTP/TCP/DNS
проверок не нужны. В шаблоне Agent capabilities сброшены; ICMP требует отдельной
проверки прав ping socket или настройки CAP_NET_RAW. Привилегии по умолчанию
ради ICMP не выдаются: [семантика и права](docs/architecture/network-probes.md).

Ограничьте 9443 адресами Agents на межсетевом экране; учитывайте правила Docker
для опубликованных портов. Не направляйте агентский gRPC через обычный HTTP proxy.
HTTP/TCP/ICMP targets сейчас задаются IP-адресами; DNS `queryName` может быть
доменом. `127.0.0.1` в YAML Agent означает сам контейнер Agent, а не мастер или host.

## Установка на сервер и Agents

Далее пример двух Agents: `branch=office`, `dc=server`. Замените адрес мастера
`10.20.0.10`, его DNS `netmonorc.example.com` и адрес целевого сервиса своими.
Не устанавливайте этот пример поверх существующих PKI, БД и spool.
Команды выполняются из корня checkout; `sudo` нужен для Docker и `/srv`.

### 1. Подготовить Linux и получить код

Пример пакетов для **Ubuntu 24.04 LTS**; для другой ОС используйте её пакеты
или [официальную установку Docker Engine](https://docs.docker.com/engine/install/).
На каждом узле:

```sh
sudo apt-get update
sudo apt-get install -y ca-certificates curl git openssl python3 docker.io docker-buildx docker-compose-v2
sudo systemctl enable --now docker
sudo docker version
sudo docker compose version
sudo docker buildx version
timedatectl status

git clone --branch prototype/minimal-research --single-branch https://github.com/boby-star/NetMonOrc.git
cd NetMonOrc
git rev-parse HEAD
```

Время должно быть синхронизировано **до запуска**. Запишите commit: Controller
и Agents устанавливайте из одной сборки. Проверенные версии инструментов в среде
проверки: Docker **28.4.0**, Compose **2.40.3**, OpenSSL 3.
Для стандартного Ubuntu timesyncd включение NTP: `sudo timedatectl set-ntp true`;
если используется chrony, проверьте его существующую конфигурацию.

### 2. Собрать один image и доставить его на Agents

На мастере или отдельной сборочной машине:

```sh
sudo docker build -t netmonorc:research .
sudo docker run --rm netmonorc:research -help
sudo docker run --rm --entrypoint /usr/local/bin/netmonorc-agent netmonorc:research -help
sudo docker save -o /tmp/netmonorc-image.tar netmonorc:research
```

Передайте archive на **каждый** Agent и загрузите его там:

```sh
scp /tmp/netmonorc-image.tar operator@AGENT_HOST:/tmp/netmonorc-image.tar
ssh operator@AGENT_HOST 'sudo docker load -i /tmp/netmonorc-image.tar'
```

В production удобнее свой registry и неизменяемый tag/digest. При корпоративном
TLS proxy передавайте доверенный build CA через BuildKit secret, как описано в
[инструкции сборки](docs/deployment/research-compose.md); не отключайте TLS checks.
Например, если нужный CA уже установлен в системный bundle:

```sh
sudo docker build --secret id=build_ca,src=/etc/ssl/certs/ca-certificates.crt -t netmonorc:research .
```

Готовый image содержит Controller, Agent, CLI, admin, Blackbox и React assets;
в нём нет shell, поэтому `docker exec controller sh` не работает.

### 3. Выпустить сертификаты вне узлов приложения

Используйте свою PKI либо следующий OpenSSL-пример на доверенной машине
администратора. `scripts/dev_pki.py` для удалённых серверов не подходит:
его server SAN — только localhost/127.0.0.1, срок действия — двое суток.

Этот блок создаёт **новый** `.local/server-pki` и остановится, если каталог уже
существует. Измените DNS/IP перед выполнением. Leaf certificates действуют 90 дней;
автоматическая выдача/ротация в данном deployment не настраивается.

```sh
(
set -eu
umask 077
mkdir -p .local
mkdir .local/server-pki
cd .local/server-pki
NMO_SERVER_DNS=netmonorc.example.com
NMO_SERVER_IP=10.20.0.10
openssl req -x509 -newkey rsa:3072 -nodes -days 3650 -sha256 \
  -subj '/CN=NetMonOrc private CA' \
  -addext 'basicConstraints=critical,CA:TRUE' \
  -addext 'keyUsage=critical,keyCertSign,cRLSign' \
  -keyout ca.key -out ca.crt
openssl req -new -newkey rsa:2048 -nodes -subj '/CN=NetMonOrc Controller' \
  -keyout controller.key -out controller.csr
cat > controller.ext <<EXT
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:${NMO_SERVER_DNS},DNS:localhost,IP:${NMO_SERVER_IP},IP:127.0.0.1
EXT
openssl x509 -req -in controller.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -days 90 -sha256 -extfile controller.ext -out controller.crt
for agent_id in branch dc; do
  openssl req -new -newkey rsa:2048 -nodes -subj "/CN=${agent_id}" \
    -keyout "${agent_id}.key" -out "${agent_id}.csr"
  cat > "${agent_id}.ext" <<EXT
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
subjectAltName=URI:spiffe://netmonorc/agent/${agent_id}
EXT
  openssl x509 -req -in "${agent_id}.csr" -CA ca.crt -CAkey ca.key -CAcreateserial \
    -days 90 -sha256 -extfile "${agent_id}.ext" -out "${agent_id}.crt"
done
openssl verify -CAfile ca.crt -purpose sslserver controller.crt
openssl verify -CAfile ca.crt -purpose sslclient branch.crt dc.crt
)
```

Controller получает только `ca.crt`, `controller.crt`, `controller.key`.
Agent branch получает только `ca.crt`, `branch.crt`, `branch.key`; dc — свою пару.
Передайте их защищённым каналом на соответствующие узлы. `ca.key` оставьте в
PKI-хранилище администратора. Имя Agent берётся из единственного URI SAN
`spiffe://netmonorc/agent/<id>` и должно точно совпадать со `slots` Controller;
изменить identity одним параметром CLI нельзя.

Пример передачи файлов с PKI-машины (checkout на узлах — `~/NetMonOrc`):

```sh
ssh operator@MASTER_HOST 'umask 077; mkdir -p ~/NetMonOrc/.local/server-pki'
scp .local/server-pki/ca.crt .local/server-pki/controller.crt .local/server-pki/controller.key operator@MASTER_HOST:~/NetMonOrc/.local/server-pki/
ssh operator@BRANCH_HOST 'umask 077; mkdir -p ~/NetMonOrc/.local/server-pki'
scp .local/server-pki/ca.crt .local/server-pki/branch.crt .local/server-pki/branch.key operator@BRANCH_HOST:~/NetMonOrc/.local/server-pki/
```

Для dc выполните аналогичную передачу только `ca.crt`, `dc.crt`, `dc.key`.

### 4. Подготовить мастер и PostgreSQL

На мастере, после получения его трёх TLS-файлов в `.local/server-pki`:

```sh
sudo install -d -m 0700 -o 10001 -g 10001 /srv/netmonorc/controller/config /srv/netmonorc/controller/data
sudo install -m 0600 -o 10001 -g 10001 .local/server-pki/ca.crt .local/server-pki/controller.crt .local/server-pki/controller.key /srv/netmonorc/controller/config/
sudo install -m 0600 -o 10001 -g 10001 examples/server-http.yaml /srv/netmonorc/controller/config/catalog.yaml
sudoedit /srv/netmonorc/controller/config/catalog.yaml
```

В `catalog.yaml` замените **оба** `192.0.2.20` на адрес настоящей HTTP-цели,
доступной из обеих зон; измените порт/URL при необходимости. Этот адрес из диапазона
документации сам по себе не является работающим сервисом.

Создайте случайный пароль БД и DSN без вывода пароля в терминал/аргументы.
Команда откажется перезаписывать существующие файлы:

```sh
sudo python3 - <<'PY'
import os, secrets
from pathlib import Path
root = Path('/srv/netmonorc/controller/config')
password = secrets.token_urlsafe(32)
files = {
    'postgres-password': password + '\n',
    'database.dsn': f'postgresql://netmonorc:{password}@postgres:5432/netmonorc?sslmode=disable\n',
}
if any((root / name).exists() for name in files):
    raise SystemExit('Existing database credentials preserved; stop and inspect the installation')
for name, value in files.items():
    fd = os.open(root / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as out:
        out.write(value)
    os.chown(root / name, 10001, 10001)
PY
sudo tee /srv/netmonorc/controller.env >/dev/null <<'ENV'
NMO_IMAGE=netmonorc:research
NMO_CONFIG_DIR=/srv/netmonorc/controller/config
NMO_DATA_DIR=/srv/netmonorc/controller/data
NMO_SLOTS=branch=office,dc=server
NMO_GRPC_BIND=10.20.0.10
NMO_UI_BIND=127.0.0.1
ENV
sudo chmod 0600 /srv/netmonorc/controller.env
```

`NMO_GRPC_BIND` должен быть IP, **назначенным этому серверу**. В примере БД
доступна только Controller через internal Docker bridge; `sslmode=disable`
относится только к этой локальной сети. Для внешней БД нужны отдельные защищённый
endpoint, учётная запись с правами создания/изменения схемы `nmo_*` и DSN с
`sslmode=verify-full`/доверенным CA. Agent не получает DSN и не подключается к БД.

Проверить YAML и поднять сначала БД, затем Controller:

```sh
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml config --quiet
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml run --rm --no-deps -T --entrypoint /usr/local/bin/netmonorc controller validate --config /etc/netmonorc/catalog.yaml --slots branch=office,dc=server
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml up -d --wait postgres
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml up -d controller
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml logs --tail=50 controller
```

Controller сам создаёт/обновляет схему БД при старте. До подключения обоих Agents
`unknown` — ожидаемый результат, а не ошибка установки. Bootstrap YAML импортируется
однократно: дальнейшие изменения ресурсов выполняйте через API/React/CLI,
а не редактированием уже импортированного файла. Подробности:
[bootstrap catalog](docs/deployment/bootstrap-catalog.md).

### 5. Создать первого администратора и открыть UI

На мастере в интерактивном терминале:

```sh
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml exec controller /usr/local/bin/netmonorc-admin -database-file /etc/netmonorc/database.dsn -username admin
```

Введите пароль 16–256 байтов UTF-8; символы не отображаются. Повторный вызов
не сбрасывает пароль существующего администратора. Публичный `ca.crt` добавьте
в доверенные сертификаты браузера. С рабочего компьютера откройте туннель:

```sh
ssh -N -L 8443:127.0.0.1:8443 operator@10.20.0.10
```

Откройте **https://localhost:8443** и войдите. SAN localhost в приведённом
сертификате предусмотрен именно для этого адреса. Для прямого доступа задайте
`NMO_UI_BIND` равным private IP сервера и открывайте DNS-имя из сертификата.
Не используйте HTTP или отключение проверки сертификата. REST и React обслуживает
сам Controller; отдельные nginx, Node runtime и frontend-сервер не обязательны.

### 6. Установить Agent в каждой зоне

На branch-узле должны быть image из шага 2 и только его CA/cert/key.
Пример предполагает, что файлы получены в `.local/server-pki`:

```sh
sudo install -d -m 0700 -o 10001 -g 10001 /srv/netmonorc/agent/config /srv/netmonorc/agent/data
sudo install -m 0600 -o 10001 -g 10001 .local/server-pki/ca.crt /srv/netmonorc/agent/config/ca.crt
sudo install -m 0600 -o 10001 -g 10001 .local/server-pki/branch.crt /srv/netmonorc/agent/config/agent.crt
sudo install -m 0600 -o 10001 -g 10001 .local/server-pki/branch.key /srv/netmonorc/agent/config/agent.key
sudo tee /srv/netmonorc/agent.env >/dev/null <<'ENV'
NMO_IMAGE=netmonorc:research
NMO_CONFIG_DIR=/srv/netmonorc/agent/config
NMO_DATA_DIR=/srv/netmonorc/agent/data
NMO_CONTROLLER=10.20.0.10:9443
NMO_SERVER_NAME=netmonorc.example.com
NMO_ALLOW_CIDRS=10.20.0.20/32
ENV
sudo chmod 0600 /srv/netmonorc/agent.env
sudo docker compose --env-file /srv/netmonorc/agent.env -f deployment/agent.compose.yaml config --quiet
sudo docker compose --env-file /srv/netmonorc/agent.env -f deployment/agent.compose.yaml up -d
sudo docker compose --env-file /srv/netmonorc/agent.env -f deployment/agent.compose.yaml logs --tail=50 agent
```

Укажите **реальный** CIDR целевого сервиса из шага 4; для нескольких сетей —
список через запятую. Это allowlist целей, а не список IP Controller. Для DNS check
в него должен входить IP resolver. `NMO_SERVER_NAME` проверяется по SAN сертификата
Controller даже при соединении по IP; не подставляйте туда имя Agent.

На dc-узле повторите шаг 6 с `dc.crt`/`dc.key` и **собственным** data-dir.
Зону определяет Controller (`dc=server`). Два процесса Agent не могут использовать
один spool; identity каталога нельзя менять с branch на dc. При одном Agent
согласованно измените и `NMO_SLOTS`, и `assessment.requiredZones`.

### 7. Проверить результат и диагностировать запуск

На мастере:

```sh
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml ps
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml exec -T postgres pg_isready -U netmonorc -d netmonorc
sudo curl --cacert /srv/netmonorc/controller/config/ca.crt -sS -o /dev/null -w '%{http_code}\n' https://localhost:8443/api/v1/auth/me
```

Последняя команда без session/PAT должна вернуть **401**: это подтверждает TLS
и доступность защищённого API, а не готовность probes. В UI проверьте «Agents»:
оба online, ожидаемые identity/зоны; затем Monitor `server-web`, свежие результаты
HTTP, достаточное coverage и `healthy`, если настоящая цель отвечает 200.
Одного `docker compose ps` недостаточно для подтверждения работоспособности.

| Симптом | Что проверить |
|---|---|
| Controller не стартует | Доступность PostgreSQL, DSN, ownership 10001, SAN/key pair, YAML и занятость портов |
| `x509` / hostname mismatch | CA, срок cert, системное время, `NMO_SERVER_NAME` и server SAN |
| Agent не виден | TCP 9443, URI SAN, `branch/dc` в slots, одинаковая версия image, журналы Agent/Controller |
| `Agent data directory must be a private directory` | mode 0700 и UID/GID 10001 каталога data |
| `already locked` / different Agent identity | Отдельный spool для каждого Agent; не удалять lock/identity для обхода проверки |
| `unknown` / неполное coverage | Оба Agents online, свежесть, время, lease, CIDR и доступность цели из каждой зоны |
| Probe execution failure | Выбранный runner, Blackbox 0.27.0, разрешения ICMP, доступность target/resolver и локальные ограничения |

Agent при разрыве связи повторяет подключение; новые probes прекращаются по
истечении lease. Spool сохраняет до 1000 ожидающих записей; при заполнении новые
проверки приостанавливаются. Сохранённая очередь не заменяет восстановление связи.

### 8. Перезапуск, обновление и резервные копии

```sh
# На мастере:
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml restart controller
# На соответствующем Agent:
sudo docker compose --env-file /srv/netmonorc/agent.env -f deployment/agent.compose.yaml restart agent
```

При обновлении сохраните backup БД и конфигурации, соберите новый versioned image,
доставьте его на все узлы, измените `NMO_IMAGE`, выполните `up -d` для Controller
и Agents и повторите шаг 7. Обычный `restart` не подхватывает новый image/env.
Один Controller обслуживает БД; HA этим шаблоном не реализуется.

Пример логического backup с мастера; файл содержит приватные данные:

```sh
(
set -eu
umask 077
mkdir -p .local/backups
backup_file=".local/backups/netmonorc-$(date -u +%Y%m%dT%H%M%SZ).dump"
set -C
sudo docker compose --env-file /srv/netmonorc/controller.env -f deployment/server.compose.yaml exec -T postgres pg_dump -U netmonorc -d netmonorc -Fc > "$backup_file"
)
```

Отдельно сохраняйте `/srv/netmonorc/controller/config`, env-файлы, PKI-хранилище
и data-dir Agents с правами доступа. Для snapshot spool сначала остановите Agent.
JSON/HTML snapshots не являются backup БД. [Восстановление лабораторного профиля](docs/deployment/backup-recovery.md)
использует другие имена projects/volumes — не запускайте его поверх серверной БД.

`docker compose ... down` без `-v` сохраняет named volume PostgreSQL и bind-каталоги.
Не удаляйте volumes, `/srv/netmonorc`, `.local` или PKI для устранения ошибок запуска.
До истечения сертификатов выпустите замену, установите её на соответствующий узел
и перезапустите процесс. Автоматический certificate renewal этим примером не обеспечивается.

## Дополнительные возможности и документация

Базовый серверный пример включает мониторинг, PostgreSQL и UI. Для диагностики
добавьте DiagnosticProfile и binding в каталог:
[декларативная диагностика](docs/deployment/declarative-diagnostics.md),
[replicas и CEL context](docs/deployment/replica-context.md).
Для alerts/webhook задайте `NMO_WEBHOOK_ALLOW_CIDRS` в controller.env и добавьте
WebhookEndpoint/AlertRule в каталог или через UI. Внешний receiver должен реально
существовать; для bearer нужны `-webhook-secret-dir` и соответствующий secretRef,
для собственного CA — `-webhook-ca` в command Controller. После изменения command/env
примените `up -d`. См. [webhook](docs/deployment/webhook-resources.md),
[зональные правила](docs/deployment/zone-alerts.md),
[персональные токены и CLI](docs/deployment/personal-api-tokens.md).

Если используются selectors Agents и подтверждения по failure domains,
дополните command Controller параметрами `-agent-labels-file` и
`-failure-domains`: [доверенный inventory](docs/deployment/check-placement.md).
Физические зоны и домены отказа задаёт администратор; установка двух контейнеров
на одном хосте не создаёт независимые failure domains.

[YAML controls](docs/deployment/yaml-controls.md),
[редактор YAML](docs/deployment/yaml-editor.md),
[batch apply](docs/deployment/yaml-web.md),
[CheckProfile](docs/deployment/check-profiles.md),
[PlacementPolicy](docs/deployment/placement-policies.md),
[assessment](docs/deployment/assessment-policies.md),
[Components / внешний exporter](docs/deployment/components.md),
[история и evidence](docs/deployment/observation-detail.md),
[экспорт диагностики](docs/deployment/diagnostic-export.md).

## Лабораторный запуск и разработка

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project netmonorc-demo
python3 scripts/lab.py up --project netmonorc-demo --ui-port 8443
python3 scripts/lab.py admin --project netmonorc-demo --username admin
python3 scripts/lab.py status --project netmonorc-demo
# Остановка с сохранением volumes:
python3 scripts/lab.py down --project netmonorc-demo
```

CA лаборатории: `.local/lab/netmonorc-demo/pki/ca.crt`; UI — https://localhost:8443.
[Контролируемые отказы](docs/deployment/research-compose.md),
[эксперименты](docs/experiments/zone-alerts-2026-11-23/README.md),
[экспорт демонстрации](docs/deployment/demo-package.md).

Для локальной разработки:

```sh
go mod download
(cd web && npm ci --ignore-scripts && npm run build)
mkdir -p .local/bin
go build -o .local/bin/netmonorc ./cmd/netmonorc
go build -o .local/bin/netmonorc-controller ./cmd/netmonorc-controller
go build -o .local/bin/netmonorc-agent ./cmd/netmonorc-agent
go build -o .local/bin/netmonorc-admin ./cmd/netmonorc-admin
go test ./...
go vet ./...
```

При запуске собранных процессов вне Docker дополнительно установите Blackbox
0.27.0 на Agents (`-blackbox-binary /absolute/path/blackbox_exporter`), настройте
private PostgreSQL/DSN, cert/key и приватные persistent directories. Controller
принимает `-http-ui-dir /absolute/path/web/dist`, `-http-listen`, `-listen`,
`-database-file`, `-slots`, `-config`, `-ca`, `-cert`, `-key`; Agent — `-controller`,
`-server-name`, `-data-dir`, `-allow-cidrs`, `-ca`, `-cert`, `-key`.
Полный список: `netmonorc-controller -help` / `netmonorc-agent -help`.
`-data-dir` относится к Agent; server-side state хранится в БД и явно заданных
snapshot-файлах. Native systemd installer в репозитории пока отсутствует.

## Что проверено в инструкции

[Проверка серверного развёртывания](docs/experiments/server-install-2026-11-24/README.md):
новые Compose-шаблоны, команды PKI/CLI, настоящий PostgreSQL, два mTLS Agents,
переход unknown → healthy, HTTPS/React assets, bootstrap и restart persistence.
Проверка выполнена на одном Docker-хосте. Установка пакетов на чистой Ubuntu,
связь физических серверов, внешний firewall и browser interactions отдельно
не подтверждены. PKI, spool и volume тестовой БД сохранены.
