# Декларативне блокування TCP-порту — 2026-11-28

Перевірено зміни поверх базової ревізії `b5ce4b4`, гілка
`prototype/minimal-research`. Контракт: [port-response](../../deployment/port-response.md).
Підсумки є локальними перевірками реалізації, не дослідом на трьох серверах
користувача та не порівнянням ефективності IDS/IPS.

## Виконані перевірки

- `go test ./...` та `go vet ./...` — passed; Go перевірки без DSN пропускають
  позначені PostgreSQL suites, тому нова durable response перевірка виконана окремо.
- `go test -race ./internal/agent ./internal/transport -run Response` — passed.
- Сувора валідація поставленого YAML з branch/office, dc/server — passed.
  Перевірено invalid port/protocol/CIDR/TTL, unknown fields/CEL та зміну revision.
- Реальний PostgreSQL 17.6: резервування до dispatch, одна спроба на епізод,
  history, generation fences, повтор receipt та переозброєння — passed.
- Host-helper policy: explicit allowlist, durable journal/restart, stable ID,
  expiry і відсутність повторного nft apply — passed.
- Production React build — passed. Нову сторінку не перевірено браузерними кліками.

## Реальне nftables виконання

`deployment/response/test_nft_live.py` виконано в окремому Docker network
namespace, `--network none --cap-add NET_ADMIN`, без host network і host firewall.
Тестовий Debian-образ містить Python, nftables та iproute2. Адреси
198.18.0.1/.2/.3 існують тільки всередині цього namespace.

Підтверджено: джерело .2 не з'єднується з TCP 18088 після команди; джерело .3
до того самого порту й .2 до 18090 продовжують працювати. Після SIGKILL helper
блокування зберігається, після kernel TTL доступ відновлюється. Збережено
`nft-live.json`. Власна таблиця не підміняє існуючі firewall таблиці.

## Наскрізний запуск

`deployment/response/test_e2e.py` використовує справжні Controller/Agent binaries,
нову тестову PostgreSQL БД, окрему dev PKI та mTLS. Agent працює під UID/GID
10001, effective capabilities = 0. Helper — окремий root-процес цього тестового
контейнера; Unix peer UID перевіряється. Binaries збирались CGO_ENABLED=0.

Один Agent office; HTTP fixture спочатку повертає restricted, потім public-debug.
Regex/CEL формує down, Controller резервує подію й автоматично надсилає команду.
Незалежний TCP-клієнт із тестової адреси .2 перевіряє блокування та відновлення.
TTL зменшено до дозволених 5s тільки в тестовій копії маніфесту.

Збережений `e2e.json` підтверджує applied, API history, відновлення за TTL,
відсутність повторної команди в тому самому епізоді та нову команду після
свіжого healthy. Це не ручне додавання nft правила замість виконання Controller.

Запуск потребує власної PostgreSQL БД у мережі тестового контейнера,
`NMO_TEST_DSN`, binaries у `/test/bins`, helper/dev_pki.py/test_e2e.py у `/test`,
поставленого YAML у `/test/example.yaml`. `Dockerfile.test` призначений тільки
для ізольованих перевірок. TLS перевірки пакетів не вимикались; для build
використовувався public CA bundle як BuildKit secret.

## Межі

Не виконувались розгортання на Germany/Poland/Netherlands та фінальний browser
acceptance. Не досліджувалась продуктивність або точність на потоці реальних
атак. `applied` — підтвердження встановлення правила; повна локалізація загрози
цим не доводиться. Без helper opt-in стандартний Agent не змінює firewall.
Невизначений результат не запускає автоматичний повтор чи продовження TTL.

Усі тимчасові контейнери/БД створені окремо для цього запуску. Раніше створені
`.local`, PKI, Docker volumes та сервіси не видалялись.
