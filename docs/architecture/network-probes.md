# Мережеві перевірки мінімального прототипу

Підтримуються HTTP/HTTPS, TCP, DNS (UDP/TCP) та ICMP через
Blackbox Exporter **0.27.0**. `native-http` залишається лише HTTP runner.
Відсутній runner і `auto` обирають Blackbox. Приклад: `examples/network.yaml`.

Кожен тип має власний закритий decoder/validator у registry компілятора.
Поля HTTP у TCP, невідомі параметри, неправильні типи, порти та regex
відхиляються. Defaults DNS: A, udp, NOERROR; ICMP: ip4, payload 32;
TCP: ip4; fallback сім'ї IP вимкнений. Не створено нових YAML-полів.

IR містить окремі типізовані TCP/DNS/ICMP параметри. Agent повторно
валідує IR; Controller перевіряє підтримку і runner, і типу протоколу
перед assignment. Для цього зрізу Controller та Agent треба оновлювати разом.

## Дозволені цілі

HTTP, TCP та ICMP поки виконуються лише для IP literals. Для DNS
перевіряється дозволеність саме IP:port resolver, а не queryName чи IP
з отриманого answer. DNS-відповідь не стає автоматично новою ціллю.
Metadata/link-local, multicast та unspecified адреси заборонені навіть
за широкого allowlist. Redirect і HTTP proxy недоступні.

Таким чином, повна firewall-політика для hostname targets ще не потрібна
цьому обмеженому зрізу й не оголошується реалізованою. FQDN як HTTP/TCP/ICMP
ціль поки не виконується, хоча DNS queryName має бути доменним іменем.

`expectedRecordRegex` перекладається у Blackbox
`validate_answer_rrs.fail_if_none_matches_regexp`: достатньо одного
відповідного answer record. Порожній answer не відповідає regex.
Приклад fixture повертає два A-records і перевіряє збіг саме другого.

## ICMP та права

ICMP не є універсальним доказом працездатності застосунку. У прикладі
він не required; required залишається HTTP. `minCoverage: 0.90`
допускає відсутність допоміжного ICMP, зберігаючи coverage_degraded.

Blackbox debug response для ICMP використовується лише для розрізнення
негативного probe та технічної неможливості виконання. Ввімкнені debug
рівні обох logger-ів exporter 0.27.0, щоб він включав prober logs у response.
Сирі логи не архівуються й не передаються у результаті. Відповідь обмежена 1 MiB.

Відмова відкриття сокета, permission error і нерозпізнаний випадок
дають execution_failed/unknown. Відомий timeout після очікування відповіді
або явна недосяжність мережі можуть бути негативним ICMP outcome.
Зміна формату exporter не повинна породжувати підтверджений down за здогадкою.

## Перевірки

```bash
go build -o .local/netmonorc ./cmd/netmonorc
python3 scripts/network_smoke.py --blackbox-binary /path/to/blackbox_exporter
```

Перевіряються HTTP/TCP/DNS, DNS через TCP, два answer records,
порожня відповідь, неочікуваний та явно дозволений NXDOMAIN,
закритий TCP-порт і комплексна оцінка двох локальних джерел.
Якщо хост не має ICMP-прав, позитивна ICMP-перевірка позначається SKIP,
а повернений unknown перевіряється окремо.

Для реального позитивного ICMP й відмови прав використовується окремий
ізольований контейнер без зовнішньої мережі. Firewall хоста не змінюється:

```bash
CGO_ENABLED=0 go build -o .local/netmonorc-static ./cmd/netmonorc
python3 scripts/icmp_smoke.py --blackbox-binary /path/to/blackbox_exporter
```

Повний розподілений сценарій:

```bash
go build -o .local/netmonorc-controller ./cmd/netmonorc-controller
go build -o .local/netmonorc-agent ./cmd/netmonorc-agent
python3 scripts/distributed_smoke.py --network --blackbox-binary /path/to/blackbox_exporter
```

Це ті самі mTLS, PostgreSQL, spool/replay та TTL, але з чотирма типами
checks. Позитивний ICMP залежить від середовища; окремий container smoke
перевіряє його в контрольованих умовах. Мережеві failure domains
у цьому same-host сценарії не є незалежними.
