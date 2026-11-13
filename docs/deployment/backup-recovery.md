# Збереження та відновлення дослідницького стенда

GitHub зберігає код, специфікації й опубліковані експерименти. Named volumes
зберігають локальну БД і spool, але не є незалежною резервною копією.

## Знімок PostgreSQL

На запущеному власному стенді:

```bash
python3 scripts/lab.py backup --project netmonorc-lab --output .local/backups/research.dump
```

Це pg_dump custom format з узгодженого snapshot PostgreSQL. Файл відразу
створюється 0600, попередній файл не перезаписується, незавершений видаляється.
Пароль не передається аргументами й не друкується. Dump містить назви цілей,
спостереження, діагнози, алерти та чергу доставки; зберігайте його приватно.
Передайте копію за межі тимчасового Cloud workspace власним захищеним способом.

Окремо збережіть приватний каталог `.local/lab/<project>` з ownership marker,
PKI й конфігураційними секретами. Не комітьте його в Git. Dump не містить
Agent spool та приватних ключів; без них це не повна копія інсталяції.
Строк дії сертифікатів стенда — дві доби; backup не подовжує їхню дію.

## Перевірка й відновлення

Перевірка читабельності архіву: `pg_restore --list research.dump` з PostgreSQL 17.
Це перевірка структури архіву; остаточна перевірка — відновлення в окрему БД.

1. Створіть окремий порожній research project з іншим приватним prefix.
2. Запустіть лише його PostgreSQL. Controller/Agent під час restore зупинені.
3. Передайте dump через stdin до pg_restore у цьому PostgreSQL:

```bash
docker compose -p recovery-demo exec -T postgres pg_restore -U netmonorc -d netmonorc --no-owner --no-privileges --exit-on-error < research.dump
```

Команда призначена тільки для порожньої БД цього окремого project; не
використовуйте --clean поверх єдиної робочої копії. Вкажіть NMO_NET_PREFIX
відповідно до підготовленого recovery project для Compose-команд.

4. Перевірте кількість observations, assessments і diagnostic runs через psql.
5. Підготуйте чинні сертифікати/конфігурацію й запустіть Controller та Agent.
   Старі результати не мають видаватися за свіжі: healthy потребує нових доказів.
   Перервані diagnostic runs завершуються inconclusive, budget не поновлюється.

Автоматичного destructive restore у helper немає. Відновлення даних саме
по собі не гарантує повторної доставки зовнішньому receiver: можливі повторні
webhook з тим самим event_id, які receiver має дедуплікувати.
