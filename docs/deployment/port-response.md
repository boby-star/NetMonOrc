# Декларативне тимчасове блокування порту на Agent

`Monitor.spec.responses` додає обмежену автоматичну реакцію на обчислений
Assessment. Приклад для серверів стенда: [response-block-port.yaml](../../examples/response-block-port.yaml).
За замовчуванням responses порожній, а Agent не має доступу до firewall.

```yaml
responses:
  - id: isolate-debug
    when: 'monitor.state == "down"'
    blockPort:
      agentRef: branch
      protocol: tcp
      port: 18088
      sourceCIDR: 188.137.254.31/32
      duration: 60s
```

Секція розташовується в `spec`, поруч із `checks` та `assessment`.
`when` — типізований CEL-контекст наявного diagnostic trigger: `monitor.name`,
`state`, `coverageState`, `labels`, `weightedScore` та `target`. Значення
coverageState: `sufficient`, `degraded`, `unknown`. Формування стану на основі
прикладних values виконується через `assessment.rules`, після чого responses
перевіряє обчислений стан. Це правило реагування, не виконуваний shell-код.

Обмеження схеми:

- До 8 правил, унікальні ID; `agentRef` має належати roster Controller.
- Тільки TCP, порти 1024–65535, за винятком 5432, 8443 і 9443.
- Одне джерело: IPv4 `/32` або IPv6 `/128`. Loopback, multicast, unspecified
  та IPv4-mapped IPv6 відхиляються. Діапазони й `0.0.0.0/0` не підтримуються.
- Тривалість 5–300 секунд. Політика й зміни умови входять до effective revision.
- Вказаний порт блокується **на вузлі agentRef**, не на Target іншого вузла.
  `sourceCIDR` обмежує джерело; це не вимкнення інтерфейсу чи всього порту
  для всіх користувачів.

## Порядок виконання та журнал

1. Controller зберігає Assessment із чинною ревізією. Реагування дозволене
   тільки за `down`/`degraded`, свіжою оцінкою, повним покриттям та наявними
   evidence у всіх оцінених зонах. `unknown` та помилка CEL дії не запускають.
2. Перевіряються умова CEL, підключений Agent, застосована ревізія та opt-in
   capability `response:block-port-v1`. Без helper opt-in команда не надсилається.
3. До надсилання в PostgreSQL резервується подія з Assessment, правилом,
   Agent, generation і строками виконання. Втрата кадру не створює нової спроби.
4. Типізована команда передається наявним mTLS-каналом. Agent перевіряє
   поточне призначення, lease, rule ID, власний ID та часові межі. Параметри
   порту беруться з чинного плану; довільного executable/command у кадрі немає.
5. Локальний helper повторно перевіряє host allowlist, Unix peer UID та
   ідемпотентність, зберігає резервування на диску й додає nft element із TTL.
6. Receipt зберігається з перевіркою Agent/generation. Повторний допуск того
   самого правила за цієї ревізії можливий після свіжого `healthy` із повним
   покриттям. Завершення TTL саме по собі не переозброює реакцію.

Одна спроба на правило/епізод: автоматичного повтору після помилки або
невизначеного результату немає. Це запобігає нескінченному продовженню блокування.
Нова ревізія має окремий облік. Уже встановлене правило не знімається достроково
при healthy, видаленні YAML або зупинці Controller; воно закінчується за TTL.

У боковій панелі **Захисні реакції** є журнал останніх 50 подій поточного
монітора, правило, Agent, порт/джерело, час, стан та збережені докази.
API: `GET /api/v1/monitors/{name}/responses`, scope `resources:read`, без query.
CLI: `netmonorc get responses security-exposure-demo` (підтримує `--context-file`).

Стани: `queued`, `applied`, `rejected`, `failed`, `unacknowledged`, `expired`.
`applied` означає успішне встановлення nft element, а не доведене припинення
атаки. `expired` — завершення його строку, не незалежна мережева перевірка.
Якщо відповідь helper або Agent втрачена, результат позначається невизначеним;
правило могло бути встановлено. Повтор тієї самої команди TTL не продовжує.

## Увімкнення на Germany / branch

Потрібні Linux, Python 3.10+, nftables, iproute2 і systemd. Усі команди нижче
виконуються на вузлі **Germany 188.137.224.35** з актуального checkout.
Оновіть Controller та всі Agents до цієї версії перед застосуванням YAML:
старі Agents не підтримують IR capability `response:policy-v1`.

```bash
sudo apt-get update
sudo apt-get install -y python3 nftables iproute2
sudo install -d -m 0755 /opt/netmonorc-response
sudo install -m 0755 deployment/response/helper.py /opt/netmonorc-response/helper.py
sudo install -m 0644 deployment/response/netmonorc-response.service /etc/systemd/system/netmonorc-response.service
sudo install -m 0600 /dev/null /etc/netmonorc-response.env
sudo tee /etc/netmonorc-response.env >/dev/null <<'EOF'
NMO_RESPONSE_PORTS=18088
NMO_RESPONSE_SOURCES=188.137.254.31/32
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now netmonorc-response
sudo systemctl status netmonorc-response --no-pager
```

Helper — окремий root-процес із вузьким набором Linux capabilities.
Сокет `/run/netmonorc-response/helper.sock` має root:10001 і mode 0660;
запити приймаються лише від UID 10001. Agent залишається UID/GID 10001,
без NET_ADMIN. Стан helper — `/var/lib/netmonorc-response/journal.json`.

Для звичайного запуску Agent додайте
`-response-socket=/run/netmonorc-response/helper.sock`.
Для контейнера додайте [agent.override.yaml](../../deployment/response/agent.override.yaml)
до **чинного** Compose запуску Agent; overlay монтує тільки каталог сокета й
встановлює `NETMONORC_RESPONSE_SOCKET`. Він не додає контейнеру NET_ADMIN.

При використанні нашого інсталятора можна зберегти overlay у його конфігурації:

```bash
sudo bash /srv/netmonorc/agent/install.sh agent --action update \
  --compose-override "$PWD/deployment/response/agent.override.yaml"
```

Цей приклад передбачає стандартний каталог установки; за іншого `--install-dir`
використайте свій шлях. Оновлення образів описано в основному README: новий
образ має бути доступний на кожному вузлі. Недостатньо оновити лише YAML.
Сертифікати, spool, БД та volumes для цієї функції перевипускати/очищати не треба.

Helper створює лише власну таблицю `inet netmonorc_response`. Вона використовує
prerouting до DNAT, обмеження локальних адрес призначення, джерела й TCP-порту;
може обмежувати також опублікований Docker-порт зі збіжним вхідним номером.
Інші таблиці не очищаються. Набір локальних адрес фіксується під час першого
створення таблиці; динамічна зміна адрес потребує окремого обслуговування.
Уже наявні правила інших firewall продовжують діяти. Rootless UID mapping і
SELinux можуть вимагати адаптації socket mount/UID; перевірений профіль —
rootful Docker на Linux, UID 10001.

Для негайного ручного скасування **всіх правил тільки цього helper** адміністратор
може зупинити сервіс і видалити його власну таблицю:

```bash
sudo systemctl stop netmonorc-response
sudo nft delete table inet netmonorc_response
```

Не використовуйте `nft flush ruleset`. Звичайна зупинка helper не знімає активні
елементи; їх видаляє ядро після завершення TTL, навіть після crash процесу.

## Кібербезпековий приклад для записки

На Germany запустіть окремий синтетичний сервіс, без реальних секретів:

```bash
python3 examples/response-service.py
```

На обох Agents потрібен probe allowlist із `188.137.224.35/32` і доступ до
TCP 18088. Це окрема вимога, не allowlist helper. Наявні дозволені CIDR збережіть.
Дочекайтеся HTTP 200 із `exposure=restricted` на обох точках спостереження.
Застосуйте `examples/response-block-port.yaml` через пакетний редактор або CLI,
дочекайтеся `healthy` та відкрийте **Захисні реакції** для `security-exposure-demo`.

У другому терміналі на Germany створіть тільки навчальний сигнал:

```bash
touch /tmp/netmonorc-demo-exposed
```

Тіло HTTP 200 зміниться на `exposure=public-debug`; regex/CEL переведе оцінку
до down. За повного покриття Controller запустить `isolate-debug`. На Germany
TCP 18088 буде тимчасово недоступний **з Netherlands 188.137.254.31**.
Перевірте з Netherlands:

```bash
curl --max-time 3 http://188.137.224.35:18088/
```

Після applied цей запит має завершитися timeout. Стан може залишатися down
або degraded: локалізація доступу не є усуненням небезпечної конфігурації.
Для відновлення безпечного вмісту на Germany:

```bash
rm /tmp/netmonorc-demo-exposed
```

Після завершення 60s TTL перевірте доступ із Netherlands повторно й дочекайтеся
свіжого healthy. Для рисунків збережіть unsafe Assessment із причиною,
запис applied з evidence/часом і результати мережевої перевірки до/після TTL.
Зміна маркера моделює порушення політики; це не доведення реальної атаки й не
повноцінна IDS/IPS. Діагностичний клас не є окремим входом responses: умова
цього етапу ґрунтується на Assessment і його CEL-контексті.

## Перевірка

`deployment/response/test_helper.py` перевіряє local allowlist, журнал,
перезапуск, повтор ID, незмінність TTL та відхилення довільних полів.
Go suites перевіряють YAML/IR, generation fences, durable admission,
актуальне призначення й Unix helper client.

`test_nft_live.py` запускається **лише в окремому container network namespace**:
перевіряє реальне блокування, незмінність інших портів/джерел і відновлення
після SIGKILL helper. `test_e2e.py` додатково запускає справжній Controller,
PostgreSQL і mTLS Agent під UID 10001, очікує автоматичну реакцію, API history,
закінчення TTL, відсутність повтору й переозброєння після healthy.
Тестовий образ — `deployment/response/Dockerfile.test`; binaries і test scripts
монтуються в `/test`, DSN передається через `NMO_TEST_DSN` у власну тестову БД.
Ці скрипти не слід запускати в host network namespace.
