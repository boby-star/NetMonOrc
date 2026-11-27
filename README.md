# NetMonOrc

NetMonOrc — исследовательский прототип комплексного мониторинга: YAML →
Controller → gRPC/mTLS → Agent → Observation → Assessment → REST/React.
Реализованы несколько Monitor, декларативные ресурсы, CEL/extraction,
Static/Fixed/Adaptive диагностика, зональные alerts, webhook, users/PAT.
Полный объём старого ТЗ ещё не реализован; актуальные ограничения находятся в
[IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md).

[Обновлённая админка](docs/deployment/admin-ui.md): боковая навигация, Manrope,
desktop/mobile, защита черновиков и браузерная проверка действий.

[Пример Adaptive для магистерской записки](examples/diagnostics/screenshot-demo/README.md):
отдельный YAML, привязка branch/dc, четыре управляемые неисправности и инструкция
для скриншота реальной диагностической сессии.

## Варианты запуска

- **Сервер и отдельные Agents:** единый [install.sh](install.sh), инструкция ниже; внутри используются
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
Установщику нужны Bash, Python ≥3.10 (только стандартная библиотека), OpenSSL и Docker/Compose. Git нужен только для checkout/сборки, на Agents он не обязателен.
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

## Быстрая установка одним bash-скриптом

Один самостоятельный `install.sh` обслуживает роли **master** и **agent**.
Он проверяет параметры и cert/key, создаёт private config/data, Compose/env,
случайный пароль БД и DSN, запускает сервисы. Повторный запуск использует
сохранённые параметры; БД, credentials, spool и bootstrap YAML не сбрасываются.
В установленном каталоге остаётся копия скрипта — checkout для управления не нужен.

Пример: Controller `10.20.0.10`, TLS DNS `netmonorc.example.com`, Agents
`branch=office`, `dc=server`, HTTP-цель `10.20.0.20:8080`.
Замените эти значения на реальные. Сертификаты должны быть уже выданы:
[рецепт PKI ниже](#подготовка-сертификатов-если-нет-своей-pki).
CA private key не передаётся в приложение или на Agents.

### 1. Получить скрипт и image

На сборочной машине/мастере:

```sh
git clone --branch prototype/minimal-research --single-branch https://github.com/boby-star/NetMonOrc.git
cd NetMonOrc
bash install.sh --help
```

Для первой сборки используйте `--build` из checkout. Готовый image можно
разместить в своём registry с versioned tag/digest и передавать через `--image`:
тогда на Agents нужны только скрипт и собственные TLS-файлы.
Никакой публичный официальный image NetMonOrc здесь не предполагается.

Если Docker/Python/OpenSSL ещё не установлены, на **Ubuntu 24.04** добавьте к
первой команде установки `--install-deps`. Этот явный флаг устанавливает
`ca-certificates openssl python3` через apt; если Docker отсутствует — также
`docker.io docker-buildx docker-compose-v2`. Существующий Docker CE/vendor Engine
не заменяется; недостающие плагины для него установите из соответствующего
репозитория. Флаг включает Docker через systemd. На других ОС установите зависимости
самостоятельно. Сам установщик не меняет firewall или службу синхронизации времени.
Перед запуском проверьте NTP: `timedatectl status`.

### 2. Установить master

Подготовьте каталог [examples/server-http.yaml](examples/server-http.yaml):
замените оба `192.0.2.20` адресом HTTP-цели, доступной из всех нужных зон.
Пример использует `native-http`; loopback в YAML относится к контейнеру Agent.
Затем **одной командой** на мастере:

```sh
sudo bash install.sh master \
  --build \
  --catalog examples/server-http.yaml \
  --slots branch=office,dc=server \
  --grpc-bind 10.20.0.10 \
  --server-name netmonorc.example.com \
  --ca /path/to/ca.crt \
  --cert /path/to/controller.crt \
  --key /path/to/controller.key \
  --bootstrap-admin
```

`--bootstrap-admin` запрашивает первый пароль скрыто в терминале (16–256 UTF-8
байтов). Для автоматизации — `--bootstrap-admin --password-stdin` и защищённый
pipe, без пароля в аргументах/env. Повторный bootstrap не меняет существующий пароль.
Можно создать администратора позже через тот же скрипт:

```sh
sudo bash /srv/netmonorc/master/install.sh master --action admin
```

Вместо сборки можно передать `--image registry.example.com/netmonorc:VERSION`.
Для корпоративного build CA: `--build --build-ca /path/to/trusted-ca-bundle.crt`.
`--install-deps` при необходимости добавляется к той же команде master.
Установщик не выпускает сертификаты и не меняет CA самовольно.

По умолчанию используются `/srv/netmonorc/master`, Compose project
`netmonorc-master`, gRPC 9443 и UI 8443. `--grpc-bind` должен быть IP этого хоста;
без него gRPC доступен только на 127.0.0.1. UI остаётся на loopback, если
не задан `--ui-bind`. PostgreSQL не публикуется на host, его volume постоянный.
DSN/пароль лежат в private config, файлы приложения принадлежат UID 10001;
данные Agent имеют mode 0700. `installation.json` хранит параметры без секретов.

Master считается запущенным после ответа HTTPS API с проверкой CA/hostname.
Это **не** утверждение о healthy целей: до подключения Agents ожидается unknown.

### 3. Установить Agents

Если registry не используется, один раз экспортируйте image и доставьте его
на узлы вместе со скриптом. Пример для branch:

```sh
(umask 077; sudo docker save netmonorc:research > /tmp/netmonorc-image.tar)
scp install.sh /tmp/netmonorc-image.tar operator@BRANCH_HOST:~/
```

Передайте этому узлу только `ca.crt`, `branch.crt`, `branch.key`. На branch:

```sh
sudo bash install.sh agent \
  --image netmonorc:research \
  --image-archive /home/operator/netmonorc-image.tar \
  --instance branch \
  --controller 10.20.0.10:9443 \
  --server-name netmonorc.example.com \
  --allow-cidrs 10.20.0.20/32 \
  --ca /home/operator/netmonorc-tls/ca.crt \
  --cert /home/operator/netmonorc-tls/branch.crt \
  --key /home/operator/netmonorc-tls/branch.key
```

Если используется registry, замените image на свой и уберите `--image-archive`.
Если image уже загружен через `docker load`, архив тоже не нужен. `--build`
доступен и для Agent, но для большого числа узлов лучше переиспользовать один image.
На Ubuntu 24.04 можно добавить `--install-deps`.

На dc повторите команду с `--instance dc` и `dc.crt/dc.key`. Identity берётся из
URI SAN, а не из имени контейнера. `--instance` можно не задавать при первой
установке: скрипт получит его из cert. Зону задаёт master (`dc=server`).

Allowlist — это CIDR **целей**, а не Controller; несколько сетей задаются через
запятую. Для DNS check включите IP resolver. Установщик проверяет TLS endpoint
Controller до запуска Agent. Сообщение «Agent started» означает успешный запуск
процесса; регистрацию, assignment и coverage проверьте на master.

По умолчанию каждый Agent получает собственные пути/project:
`/srv/netmonorc/agent-branch`, `netmonorc-agent-branch`;
`/srv/netmonorc/agent-dc`, `netmonorc-agent-dc`. Несколько Agents на одном хосте
не делят spool. Их можно устанавливать одним и тем же скриптом с разными cert/id.

### 4. Открыть UI и проверить установку

Добавьте публичный CA в доверенные сертификаты браузера и с рабочего компьютера
откройте туннель (сертификат из примера включает localhost):

```sh
ssh -N -L 8443:127.0.0.1:8443 operator@10.20.0.10
```

Откройте **https://localhost:8443**. В UI проверьте Agents online, правильные
identity/зоны, свежие результаты, достаточное coverage и healthy при ответе цели
200. При прямом доступе используйте DNS из server SAN и нужный `--ui-bind`.
TLS verification не отключается. Отдельный Node/nginx для UI не требуется.

```sh
sudo bash /srv/netmonorc/master/install.sh master --action status
sudo bash /srv/netmonorc/master/install.sh master --action logs
sudo bash /srv/netmonorc/agent-branch/install.sh agent --instance branch --action status
sudo bash /srv/netmonorc/agent-branch/install.sh agent --instance branch --action logs
```

При custom `--install-dir` передавайте его и при дальнейшем управлении.

| Симптом | Проверка |
|---|---|
| Нет Docker/Compose | Зависимости и доступ к локальному rootful daemon; Ubuntu 24.04: `--install-deps` |
| Ошибка TLS | CA, cert/key, срок, время, server DNS SAN и `--server-name`; Agent URI SAN |
| Agent не online | TCP 9443, его id в `--slots`, одинаковый image, журналы обоих узлов |
| Unknown/coverage degraded | Все required zones, NTP ≤5 секунд, lease, allowlist, доступность цели из каждой зоны |
| Identity mismatch | Чужой cert не должен использовать существующий spool; выдайте правильную пару |
| Existing unmanaged directory/project | Не перезаписывается; используйте прежний механизм управления или отдельную согласованную миграцию |

### 5. Массовая установка и добавление Agents

Один versioned image и `install.sh` можно доставлять Ansible/SSH/системой
управления конфигурацией; каждому узлу передаются только его cert/key,
Controller address и allowlist. Пароли БД или пользовательские PAT Agent не нужны.
Для подготовки конфигурации без запуска/доступного Controller есть `--no-start`;
повторите команду без этого флага после подготовки сети. Registry credentials
настраиваются для root через `sudo docker login REGISTRY`, не через параметры установщика.

Чтобы добавить branch2 в office, выдайте ему cert с URI
`spiffe://netmonorc/agent/branch2`, затем обновите roster master:

```sh
sudo bash /srv/netmonorc/master/install.sh master --action update \
  --slots branch=office,branch2=office,dc=server
```

После этого выполните обычную команду Agent с `--instance branch2` и его cert/key.
Планируемое покрытие/PlacementPolicy настройте через API/CLI/React; новая зона
не становится автоматически required. Текущий Controller поддерживает **до 100
Agent identities** в roster. Автоматический discovery/enrollment этим скриптом
не добавляется. Масштабирование Agents не означает HA Controller: один master
обслуживает свою БД. Независимые master-инсталляции должны иметь разные
`--install-dir`, `--project` и порты при размещении на одном хосте.

### 6. Повторный запуск, обновление и смена сертификатов

Повторная установка с теми же параметрами не пересоздаёт credentials/БД/spool.
Сохранённые значения используются, если флаги опущены:

```sh
sudo bash /srv/netmonorc/master/install.sh master
sudo bash /srv/netmonorc/agent-branch/install.sh agent --instance branch
```

Для обновления используйте новую версию `install.sh` и versioned image:

```sh
sudo bash install.sh master --action update --image registry.example.com/netmonorc:NEW_VERSION
sudo bash install.sh agent --instance branch --action update --image registry.example.com/netmonorc:NEW_VERSION
```

Для custom каталога укажите `--install-dir`. Отсутствующий image загружается;
`--pull` явно обновляет уже локально существующий tag. Для offline-обновления
доступен `--image-archive`. Обновляйте Controller и Agents согласованно.
Изменения private configuration сохраняются в private `history/` до применения;
автоматического rollback схемы PostgreSQL нет. При ошибке readiness конфигурация
и данные остаются на месте, смотрите `--action logs`.

Сертификаты обновляются только явно (Agent identity остаётся прежней):

```sh
sudo bash install.sh agent --instance branch \
  --ca /path/to/ca.crt --cert /path/to/new-branch.crt --key /path/to/new-branch.key \
  --rotate-tls
```

Смена cert/key вызывает пересоздание соответствующего контейнера. Для master
доступен тот же `--rotate-tls`; миграцию CA планируйте с учётом всех Agents.
Автоматического certificate renewal нет. Bootstrap `--catalog` не перезаписывается:
для изменения работающих ресурсов используйте API/CLI/React.

Остановка сохраняет контейнеры, сети, БД и очередь:

```sh
sudo bash /srv/netmonorc/agent-branch/install.sh agent --instance branch --action stop
sudo bash /srv/netmonorc/master/install.sh master --action stop
```

Установщик не выполняет `down -v`, prune или очистку PKI/data. Чужие unmanaged
каталоги и Compose projects/volumes не присваиваются. Существующие ручные установки
из предыдущего README не мигрируются автоматически; их
[Compose-шаблоны](deployment/server.compose.yaml) остаются доступными.
До переноса существующей БД в новый project сделайте backup и план восстановления.

### 7. Backup

Для установленного master (root, приватный dump, отказ при существующем имени):

```sh
sudo bash -c '
set -eu
umask 077
mkdir -p /srv/netmonorc/backups
set -C
docker compose -p netmonorc-master --env-file /srv/netmonorc/master/compose.env \
  -f /srv/netmonorc/master/compose.yaml exec -T postgres \
  pg_dump -U netmonorc -d netmonorc -Fc \
  > "/srv/netmonorc/backups/netmonorc-$(date -u +%Y%m%dT%H%M%SZ).dump"
'
```

Для custom project/path подставьте сохранённые значения. Отдельно сохраняйте
private installation directory с config, `installation.json`, `compose.env`,
history и PKI; spool копируйте после остановки Agent. JSON snapshots не являются
backup БД. Данные должны храниться на постоянном диске. Spool ограничен 1000
ожидающих записей; автоматическая retention БД и backup scheduler не реализованы.

## Подготовка сертификатов, если нет своей PKI

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

Пример передачи файлов с PKI-машины (каталог назначения на узлах — `~/netmonorc-tls`):

```sh
ssh operator@MASTER_HOST 'umask 077; mkdir -p ~/netmonorc-tls'
scp .local/server-pki/ca.crt .local/server-pki/controller.crt .local/server-pki/controller.key operator@MASTER_HOST:~/netmonorc-tls/
ssh operator@BRANCH_HOST 'umask 077; mkdir -p ~/netmonorc-tls'
scp .local/server-pki/ca.crt .local/server-pki/branch.crt .local/server-pki/branch.key operator@BRANCH_HOST:~/netmonorc-tls/
```

Для dc выполните аналогичную передачу только `ca.crt`, `dc.crt`, `dc.key`.

## Дополнительные возможности

Базовый пример включает мониторинг, PostgreSQL и React. Диагностику добавляйте
через DiagnosticProfile/binding в bootstrap или API:
[декларативная диагностика](docs/deployment/declarative-diagnostics.md),
[replicas/CEL](docs/deployment/replica-context.md).
Для webhook используйте `--webhook-allow-cidrs`, ресурсы WebhookEndpoint/AlertRule
и реально доступный HTTPS receiver. Для custom command (bearer secret-dir, receiver
CA, trusted inventory/failure domains) передайте `--compose-override /path/to/override.yaml`.
Установщик сохраняет его как `compose.override.yaml` и использует при всех действиях
и обновлениях. Compose заменяет command-массив целиком: укажите полный command с
дополнительными flags и нужные mounts. Host-пути в override задавайте абсолютными. Base `compose.yaml`/`compose.env` генерируются
установщиком; custom изменения размещайте в override, а не в base-файлах.
[Webhook](docs/deployment/webhook-resources.md), [зональные alerts](docs/deployment/zone-alerts.md).

[YAML controls](docs/deployment/yaml-controls.md), [редактор](docs/deployment/yaml-editor.md),
[batch apply](docs/deployment/yaml-web.md), [PAT/CLI](docs/deployment/personal-api-tokens.md),
[Agent inventory/placement](docs/deployment/check-placement.md).
Доверенные Agent labels и failure domains задаются master, а не самими Agents.

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

## Проверка установщика

[Результаты](docs/experiments/installer-2026-11-24/README.md): реальный PostgreSQL,
master и два Agents, mTLS/HTTP до healthy, повторная установка, update image,
сохранение сессии/spool, отказ при неверных CIDR/identity/unmanaged directory.
Проверка выполняется на одном Docker-хосте через root driver; это не испытание
между физическими серверами. Установка apt-пакетов на чистой Ubuntu, browser
interactions, HA, автоматическая выдача сертификатов отдельно не подтверждены.
