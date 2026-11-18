# Відомі домени відмови для confirmation

Підстава — §2.1 та §2.3 «Чернов С. записка v1.docx»: кілька Agent можуть мати
спільний роутер, а невідомий домен не можна рахувати незалежним джерелом.
Controller тепер використовує окреме trusted призначення доменів; networkZone
та Agent ID не підміняють це знання.

## Налаштування

```sh
netmonorc-controller ... -slots=branch=office,dc=server \
  -failure-domains=branch=office-path,dc=server-path
```

Domain ID — символічне ім'я залежності: 1–63 символи, починається з малої
латинської літери, далі малі літери/цифри/дефіс. Agent має існувати в slots;
дублікати Agent, порожні/невалідні домени та невідомі Agent відхиляються до
підключення до БД. Кілька Agent **можуть** мати однаковий domain ID.

Спільний шлюз у різних зонах описується явно:

```sh
-failure-domains=branch=shared-gateway,dc=shared-gateway
```

Для відсутнього mapping домен невідомий. Два різні Agent у різних зонах із
таким mapping дають один відомий домен, а без mapping — нуль. Вимога
confirmation.minDistinctFailureDomains=2 тоді не виконується: висновок
залишається inconclusive, навіть коли конкретна гіпотеза підтримана іншими
результатами. Самі валідні probe outcomes та Assessment не перетворюються на
execution failure через відсутність відомого домену.

## Сумісність і історія

Це зміна безпеки висновків: старі команди запуску без failure-domains більше
не отримують автоматичне підтвердження незалежності з назв зон. Compose
дослідницького стенда й distributed_smoke задають домени своїх логічних шляхів
явно. Ці шляхи мають спільний фізичний Docker-хост; різні domain IDs не є
математичним доказом незалежності чи незалежного обладнання.

Mapping задається оператором Controller до запуску та не приймається з
AgentHello, heartbeat або Observation. Зміна потребує перезапуску Controller.
Початкові регулярні докази з probe, початого до запуску цього trusted mapping,
не використовуються для нового діагнозу: потрібні свіжі результати. Так старий
доказ не набуває нового домену заднім числом. Це консервативний epoch guard,
а не новий DSL kind чи зміна source resourceVersion Monitor.

Кожний новий diagnostic Decision зберігає failureDomain виконавця;
Confirmation зберігає evidenceDomains — mapping зарахованих proof IDs до
доменів. Порожній рядок означає невідомий домен. Legacy reports не доповнюються
поточними значеннями. Дані доступні в GET diagnostic detail, CLI та evidence
export; React показує домени рішень/доказів. Agent API/React показує поточний
Controller-owned failureDomain навіть для offline Agent; відсутній — «Невідомий».

## Межі

Статичне trusted призначення підтримує дослідницький confirmation. Воно не
додає full Agent CRUD, enrollment, автоматичне вивчення топології або
placement distinctBy=failureDomain. Кілька рівнів залежностей потребують
окремої моделі; один domain ID представляє явно вибрану спільну залежність.

Критичні перевірки: parse/roster validation, shared/unknown не стають різними
доменами, збереження історичних доменів, epoch guard для baseline. Live сценарій:
`scripts/lab_failure_domains.py` на новому prepared/up project без користувачів.
