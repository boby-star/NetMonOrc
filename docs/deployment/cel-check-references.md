# Перевірка імен checks у декларативних CEL-правилах

DSL SC-18 вимагає відхиляти посилання на неіснуючий check під час semantic
validation. Тепер AssessmentRule перевіряє статично задані імена у типізованому
CEL AST, до створення desired revision або передавання Agent конфігурації.
Раніше `checks["htttp"].outcome == "failure"` проходив компіляцію і давав лише
runtime evaluation error замість раннього повідомлення про помилку YAML.

Перевіряються `checks["id"]`, `checks.id`, `has(checks.id)` та `"id" in checks`,
включно з вкладеними списками, об'єктами, macro й недосяжними гілками. Імена
порівнюються з кінцевими Plan.checks після CheckProfile expansion. Змінна macro,
що локально має назву checks, не вважається глобальним контекстом Monitor.
Повідомлення вказує ID правила й невідомий check. Збір імен упорядкований,
тому за кількох помилок вибір повідомлення не залежить від Go map order.

У повторно використовуваній AssessmentPolicy з явним списком checks набір
імен відомий відразу. Без цього списку політика може бути створена окремо;
компіляція перевіряє форму і типи на синтетичних даних, а конкретні імена
перевіряються при прив'язуванні до Monitor. Синтетичні checks не додаються
до source, executable Plan чи live coverage. Зміна політики проходить спільну
валідацію її залежних Monitor. Ліміти 100 правил, 100 checks та 4096 bytes на
CEL expression застосовуються і до цього попереднього етапу.

[Приклад із трьох YAML-документів](../../examples/cel-check-references.yaml):
CheckProfile http з alias web утворює check web__http. Звернення до http або
web__htttp у такому Monitor буде відхилено. Offline перевірка:

```sh
netmonorc validate --config examples/cel-check-references.yaml --slots branch=office
```

Це спільний validator для Controller bootstrap, offline/remote CLI, REST та
YAML-редактора React. Нового API або поля маніфесту немає. REST validate повертає
valid=false; plan/apply і прямий запис ресурсу відхиляють некоректний Monitor.
Попередня ревізія і source version залишаються чинними.

Динамічні ключі, наприклад checks[id] у macro, не розв'язуються статично.
Так само ця перевірка не обчислює вирази, що конструюють рядкові ключі.
Їхні runtime errors, відсутні metrics/extracted values, TTL/coverage і cost
limits обробляються наявними механізмами CEL. Наявність check у Plan не означає,
що свіжий Observation чи необов'язкове значення вже існують.

[Натурний запуск і межі](../experiments/cel-check-references-2026-11-22/README.md).
