# Короткий запис Monitor.spec.dependencies

Поле підтримує масив імен Target, наприклад:

```yaml
spec:
  targetRef: application
  dependencies: [corporate-dns, gateway]
```

[Повний YAML](../../examples/monitor-dependencies.yaml) можна подати у Controller
bootstrap, CLI validate/plan/apply або редактор React. Цілі можуть бути в тому
самому пакеті після Monitor. Їхня наявність перевіряється до запису desired state.

## Семантика

Застосовано детальне правило DSL §15.1: `Monitor/name → Target/ref`, relation
requires, strength soft. Для inline target джерелом теж є Monitor, без створення
прихованого Target. Стисла таблиця §4.1 згадує inline/linked target; цей release
використовує конкретне визначення §15.1, як раніше рекомендовано в dependencies.md.
Для іншого джерела, relation, strength або опису використовуйте явний Dependency.

Короткий запис утворює похідні links, а не ресурси БД. Якщо є явний requires для тієї
самої пари, він має пріоритет: його name/strength/description збережено, додатковий
soft link не створюється. Інші relations для тієї самої пари не підміняють requires.
Два явні Dependency й далі мають попередню семантику; цей механізм їх не зливає.

Вихідні й транзитивні links входять у той самий dependency context IR та
перевіряються Agent. Залежність залишається описовим контекстом: відсутність
Observation для logical Target не робить Monitor down, strength не переносить
стан і граф не є доказом першопричини. Нові probes для перелічених Target не
створюються автоматично.

## Валідація, hash та зміни

- Значення — масив унікальних чинних Target names. Null, об'єкти, числові
  елементи, дублікати й невідомі refs відхиляються. Ліміт — до 4096 refs на
  Monitor та до 4096 links у всьому графі, з чинним transport budget Plan.
- Requires DAG перевіряється спільно для явних і похідних links. Цикл через
  Dependency у зворотному напрямку теж блокує весь apply.
- Порядок refs не змінює effective hash. Derived IDs стабільні за парою refs.
  Порожній dependencies: [] еквівалентний відсутньому полю й зберігає старий hash.
  Порядок у збереженому source не переписується compiler-ом.
- Target не можна видалити, доки збережений Monitor його перелічує. DELETE
  повертає 409. Спочатку змініть Monitor; після вилучення посилання Target можна
  видалити, якщо інших refs немає. Registry/deletion використовують той самий
  writer lock, що atomic apply.
- Вилучення Monitor або списку прибирає похідні links; окремих Dependency для
  очищення немає. Історичні Plan та Observation не переписуються.

## Перегляд

GET `/api/v1/resources/Monitor/{name}/dependencies` — read-only resolved desired
context: monitor, configRevision, items. Кожен item має from/to/relation/strength,
name, derived та declaredBy. Для похідного link declaredBy=Monitor/name, для
явного — Dependency/name. Це не ResourceView з вигаданим UID чи resourceVersion.
Source hash/ревізія стосуються скомпільованого desired context, не live health.
Доступ — resources:read, чинні auth/admission/timeout. Query не підтримується.
Контракт endpoint додано до OpenAPI; повну синхронізацію всіх інших нових API
цим комітом не заявлено.

CLI: `netmonorc get --context-file /private/context.json dependency-context NAME`.
React: у графі «Залежності ресурсів» натисніть «Контекст Monitor NAME». Похідні
links позначені «Неявна залежність» та редагуються через dependencies у Monitor;
кнопка «Оновити граф» лишає звичайний пагінований каталог явних Dependency.

[Натурні докази](../experiments/monitor-dependencies-2026-11-22/README.md):
Agent ACK/healthy, compiled API/CLI/React context, 409 на DELETE, rollback змішаного
циклу й можливість видалення Target після вилучення посилання.
