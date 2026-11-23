# Fixed DAG: типізовані умови й оголошені попередники

DSL §11.5 та §18 визначають when як булеву умову над завершеними попередниками
DAG. Записка §2.3–2.4 вимагає відтворюваних fixed/adaptive сценаріїв і
відокремлення недостатності доказів від негативного результату цілі.

Раніше Fixed CEL context був вкладеним map<string,string>: помилки назв
полів проходили compilation. Умова також бачила всі завершені tasks,
включно з незалежними гілками. За іншої паралельності така гілка могла ще
не завершитися, тому прихована залежність змінювала результат сценарію.

Тепер tasks[id] є типізованим FixedTaskView з outcome, executionStatus,
values та reasonCode;
monitor має лише state. Невідомі поля, зокрема outcomme/monitor.sttae,
відхиляються до збереження профілю. Значення outcome/executionStatus лишаються
рядками чинного контракту. Skipped → not_executed, execution_failed → failed,
відсутній task → evaluation error. Values/reasonCode доповнені в наступній ітерації: вони надходять із committed
Observation та зберігають scalar types. [Правила допуску й приклад](declarative-diagnostics.md).

Статичні посилання tasks["id"], tasks.id, has і membership повинні вказувати
на прямого або транзитивного попередника через dependsOn. Посилання на себе,
майбутній/невідомий чи незалежний крок відхиляється. DAG перевіряється до
компіляції умов. Сортування імен та врахування локальних змінних macro спільні
з [перевіркою AssessmentRule](cel-check-references.md).

Під час виконання і serial, і parallel engines передають кожній умові окремий
набір outcomes лише її попередників. Навіть computed key або перебір tasks
не відкриває результати незалежної гілки. Це не розв'язує computed keys
статично: звернення до відсутнього ключа дасть runtime evaluation error,
крок не виконається, діагностика зафіксує недостатній контекст. Звичайне false
залишається умовним пропуском. Моніторинг і fallback Assessment не змінюються.

Приклад: [fixed-context.yaml](../../examples/diagnostics/fixed-context.yaml).
Два Agent виконують незалежні probes, a-join залежить від обох, d-finish — від
a-join і читає також транзитивного попередника b-office. Кожний фізичний запуск,
включно з повторним actionRef join-http, витрачає окрему одиницю бюджету.

```sh
netmonorc validate --config examples/diagnostics/fixed-context.yaml --slots branch=office,dc=server
```

Немає нових YAML-полів чи HTTP routes. Змінюється валідація та набір доступних
фактів умови; серіалізований Profile і revision коректних старих сценаріїв
не отримують нових полів. Старі профілі з прихованими посиланнями потрібно
виправити, додавши потрібні dependsOn; залежності не вгадуються автоматично.
Спільний compiler використовується bootstrap, REST, CLI та React.

[Підтверджений запуск і межі](../experiments/fixed-context-2026-11-22/README.md).
