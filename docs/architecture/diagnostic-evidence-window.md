# Часова придатність діагностичних доказів

Підстава — §2.1–2.3 «Чернов С. записка v1.docx», редакція SHA256
`df5f9d1f6154a65713b48455d58a69c17e961f2eabab61e359f8d3610d184d8c`:
результати одного висновку мають відповідати maxSkew; після завершення TTL
або порушення часового узгодження робочий набір переглядається, відкинуті
гіпотези можуть відновитися, а витрачений бюджет не повертається.

## Реалізація

Controller передає в Engine наявні Monitor freshness.maxAge та maxSkew.
Нових YAML полів/алгоритмів вибору не додається. Для одноразового diagnostic
probe без власного interval/TTL використовується Monitor maxAge. Початковий
регулярний доказ зберігає строк придатності свого check, визначений раніше.
Усі режими Static/Fixed/Adaptive отримують однакову runtime-політику.

Runner повертає outcome разом з ID і фактичним finishedAt нормалізованого
Observation **після його durable commit**. Час надходження відповіді не
замінює час probe. Абсолютна межа validUntil для додаткового результату:
мінімум із finishedAt + maxAge та спільного session deadline.

Перед кожним вибором/після нового результату/перед поверненням висновку:

1. Обирається останній результат кожної пари action–Agent в послідовності
   фізичних активацій. Його непридатність не повертає старіший зручний результат.
2. Відкидаються technical execution failures, відсутні/майбутні timestamps,
   прострочені докази. Unknown не підміняється negative outcome.
3. З решти формується вікно maxSkew від найновішого finishedAt.
4. Hypotheses та support перебудовуються з повного каталогу за цим набором.

Confirmation перевіряє той самий TTL/maxSkew і придатність останнього
результату пари. Профільні повтори можуть дати кілька реальних сумісних
підтверджень у вікні; minSpacing застосовується до physical finishedAt,
а не інтервалу доставки. Застарілі первинні proofs не доповнюють свіжі
результати заради виконання minObservations/minDistinctFailureDomains.

Після очікування admission повторно порівнюється придатний робочий набір:
зміна через TTL/час потребує нового вибору до списання бюджету. Немає
автоматичного повтору вже виконаних дій через expiry: чинні explicit
retry/confirmation та межі бюджетів зберігаються. Нестача доказів дає
inconclusive; історичні фізичні спроби залишаються в журналі.

## Пояснення через API/CLI/React

Звіт зберігає evidencePolicy (maxAgeMs/maxSkewMs), а кожне рішення —
observationId, finishedAt і validUntil, якщо є фактичний Observation.
GET detail/CLI/evidence export повертає ці поля; React показує timestamps і
політику сесії. Legacy reports не доповнюються поточними або вигаданими часами.
validUntil є верхньою межею TTL/deadline, а не гарантією входження у maxSkew
чи перебування останнім результатом своєї пари.

## Межі

Вікно базується на прийнятих часових позначках Agent; мала різниця між ними
не доводить синхронізацію годинників. Чинні транспортні перевірки походження,
revision, timestamps та clock uncertainty залишаються необхідними. Тут немає
NTP-калібрування або гарантії фізичної одночасності на різних хостах.

Звичайний Runner без часових metadata лишається доступним для автономних
каталожних simulations без ввімкненої temporal policy. Production temporal
policy потребує EvidenceRunner; успішний outcome без finishedAt не стає доказом.
Це однаковий контроль у fixed та adaptive runtime, а не зміна формули gain/cost.

Критичні regressions: coherent/incoherent conclusions, TTL і future/delayed
results, відновлення гіпотез після superseded результату, baseline + новий
probe поза maxSkew, physical spacing та збереження spent checks/cost.
Live сценарій: `scripts/lab_temporal_evidence.py` на новому owned стенді.
