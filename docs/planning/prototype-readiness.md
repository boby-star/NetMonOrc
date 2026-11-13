# Оцінка готовності мінімального прототипу

Орієнтир після confirmation, повного DNS timeout, webhook і bounded technical retry:
**приблизно 94% виконано, близько 6% залишається**. Це інженерна оцінка
узгодженого мінімуму із записки, не вимірювання кількості коду, не гарантія
приймання роботи й не оцінка повного продуктового v1alpha1.

Наведені ваги допомагають пояснити оцінку. Вони планові, а не математичні
властивості системи. Часткова готовність не означає, що кожна функція
всередині категорії реалізована частково: фактичні межі наведено в IMPLEMENTATION_STATUS.md.

| Частина мінімуму | Вага | Орієнтовно покрито | Підстава й залишок |
|---|---:|---:|---|
| YAML, модель, стандартні probes | 20 | 18 | HTTP/TCP/DNS/ICMP реальні; підмножина DSL, не всі refs/моделі |
| Розподіленість і доставка | 15 | 15 | mTLS, leases, spool/replay, generation fencing, PostgreSQL, dual-zone deployment; додано BOOTTIME guard і перевірено backup/restore |
| Комплексне оцінювання | 20 | 18 | H/Q, required, TTL/revision/maxSkew, зони, CEL, extraction; потрібна остаточна звірка зі сценаріями записки |
| Діагностичний метод | 20 | 19 | Confirmation, bounded technical retry, budget/domains/spacing, HTTP+DNS; залишаються ширші unknown/multiple patterns |
| Запуск і демонстрація | 10 | 10 | CLI, HTML зі станом алерту, Compose й durable HTTPS webhook; CI, приватний backup/restore та матриця приймання |
| Незалежні відтворювані експерименти | 15 | 14 | Повторна серія 30 запусків, 30 очікуваних результатів; потрібні ширші fault patterns і фінальна інтерпретація для записки |
| Разом | 100 | 94 ≈ 94 | Приблизний орієнтир обсягу, не метрика якості |

## Найближчі результати до завершення

1. Перенести матрицю приймання та обмеження до фінальної записки;
   не описувати Multi-replica/ResourceGroup як перевірені компоненти.
2. Перевірити довші tasks,
   backpressure/queue, suspend і неперевірені комбінації несправностей.
3. Розширити unknown/multiple patterns; повторна 30-run серія вже завершена.
   Два повтори не дають статистичних гарантій.
4. Звірити текст записки з фактичними межами прототипу. Групи/залежності,
   які не перевіряє прототип, лишаються явно позначеним розширенням.

Виправлено відомий HTTP+DNS false class за допомогою явної confirmation;
перевірено DNS silent timeout. Це не гарантія для довільної unknown/multiple
несправності, що має той самий спостережуваний pattern.

Дані: [старий пілот](../experiments/compose-pilot/README.md),
[confirmation](../experiments/confirmation-pilot/README.md),
[DNS-drop](../experiments/dns-drop-pilot/README.md),
[webhook/restart](../experiments/webhook-pilot/README.md),
[technical retry](../experiments/diagnostic-retry-pilot/README.md),
[повторна серія](../experiments/repeated-series/README.md).

Повний CRUD/RBAC/enrollment/React UI/HA та всі можливості великого v1alpha1
не є умовою завершення цього мінімального дослідницького профілю.
