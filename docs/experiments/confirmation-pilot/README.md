# Пілот явного підтвердження

Source commit: `8a5d9a8`, clean tree на початку. Metadata містить image IDs,
policy=confirmed і seed. Це 15 запусків, по одному повтору п'яти сценаріїв
для Static, Fixed і Adaptive. Фізичний стенд та той самий каталог із шести
actions/бюджет шести checks збережені. Код не отримує ground truth.

У всіх 15 семплах отримано очікуваний стан і правильну діагностичну відповідь.
HTTP+DNS тепер дає inconclusive у всіх трьох режимах. Для одиничних HTTP,
DNS, TCP та office-path отримано відповідний клас. Сирі дані — primary.json
і primary.csv; контрольні суми — sha256.json.

| Сценарій | Static | Fixed confirmed | Adaptive confirmed |
|---|---|---|---|
| HTTP | 6 probes, http-failure | 4 probes, http-failure | 4 probes, http-failure |
| DNS | 6 probes, dns-failure | 4 probes, dns-failure | 4 probes, dns-failure |
| TCP | 6 probes, tcp-failure | 4 probes, tcp-failure | 4 probes, tcp-failure |
| Office path | 6 probes, office-path-failure | 4 probes, office-path-failure | 4 probes, office-path-failure |
| HTTP+DNS | 6 probes, inconclusive | 4 probes, inconclusive | 4 probes, inconclusive |

Adaptive має дві primary та дві confirmation дії. Для відмови HTTP він
більше не покладається тільки на виключення інших класів: server HTTP
перевіряється явно. Domain count походить від зон Controller roster;
цей стенд перевіряє незалежність шляхів, не фізичних хостів.

Ціна зміни: чотири probes замість двох у ранньому Adaptive/частині Fixed
сценаріїв. Це не загальний виграш над Fixed; ціль — виправити дві помилки
попереднього пілоту. Fixed confirmed — інший явний DAG, тому покращення не
приписується одному лише engine без зміни конфігурації. Первинний Gain/Cost
не змінено; нульовий Score confirmation не означає довільну нову евристику.

Один повтор не дає статистичного доказу точності чи переваги за latency.
Невідома несправність із тим самим перевіреним pattern усе ще може бути
нерозрізнюваною. Silent drop/clock-deadline окремо потребують перевірки.
Старі [результати](../compose-pilot/README.md) не змінено й не приховано.

Відтворення: підготувати стенд нової версії, `lab.py up --policy confirmed`,
потім `lab_experiment.py --policy confirmed --repetitions 1` з новим output.
Результати різних policy/source не об'єднувати для висновків про час.
