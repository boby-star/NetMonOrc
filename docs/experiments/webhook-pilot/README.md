# Наскрізна перевірка алертів та HTTPS-доставки

Source commit `9b420e7`, чисте дерево на початку. Образи й команда в
metadata.json. Raw primary.json збережено без переформатування; SHA256SUMS
дозволяє перевірити обидва файли. Секрети/DSN/ключі не експортувалися.

Реальні Controller, два mTLS Agent, PostgreSQL і HTTPS receiver працювали
у приватному Compose-стенді без host ports. Несправність — HTTP 503
контрольованого ресурсу; окремий receiver mode file змушував сам webhook
повертати 503. Це різні типи відмов, які не змішуються в Assessment.

| Крок | Спостережений результат |
|---|---|
| HTTP-відмова ресурсу | down, після `for: 2s` — firing |
| Приймач тимчасово недоступний | HTTP 503, retry з тим самим event_id, HTTP 200 |
| Обидва Agent зупинено до expiry | Assessment unknown; алерт лишився firing |
| Controller перезапущено | firing відновлено; нового firing event немає |
| Ресурс healthy, приймач повертає 503 | resolved збережено в durable outbox |
| Controller зупинено/піднято, приймач відновлено | resolved доставлено з початковим event_id |
| Receiver-side dedup | по одному logical event firing/resolved при двох запитах кожного типу |

У цьому прогоні кожна подія мала дві завершені HTTP спроби: 503 і 200.
PostgreSQL attempt audit та receiver SQLite наведено в primary.json.
Receiver audit містить також попередній пілот робочого дерева; записи саме
цього прогону визначаються event_id у firing/resolution. Фінальний стан:
ресурс healthy з повним coverage, правило resolved.

Це один контрольний прогін. Він перевіряє конкретний шлях retry/restart,
а не доводить exactly-once або доступність довільного зовнішнього receiver.
Загальні FSM-властивості, cooldown/rearm, TLS/redirect/metadata/secret file
перевірено окремими критичними Go-тестами. Deadline/attempt exhaustion,
endpoint replacement та 24h TTL описані реалізацією, але цим натурним
прогоном не перевірялись; вони не позначені успішними тут.

Відтворення: [alert-delivery.md](../../architecture/alert-delivery.md).
