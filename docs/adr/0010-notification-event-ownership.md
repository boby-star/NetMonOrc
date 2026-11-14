# ADR 0010: Notification identities та спільний Operation envelope

Статус: рішення реалізації погодженого webhook.test без fake Alert/message.
HTTP routes та DSL fields не додаються.

Старий outbox FK вимагав alert event для кожної доставки. Notification ledger
відокремлює queue identities від Alert lifecycle. Міграція переносить чинні
IDs/payload без перевидачі; test використовує ту саму чергу та Sender.

Operation read envelope відокремлено від config.apply targets. Kind зберігається
в БД; read scope перевіряється за типом до pagination. Config reconciliation
не залежить від webhook result. Delivery transaction оновлює test progress
разом із outbox/attempts. Один pending test revision серіалізується коротким
DB lock, без нового сервісу чи брокера.
