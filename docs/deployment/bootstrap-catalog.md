# Декларативний стартовий каталог YAML

Controller приймає через `-config` один Monitor або пакет 1..20 ресурсів YAML,
розділених `---`. Це ті самі kinds та поля DSL, що у CLI validate/plan/apply.
[Повний приклад](../../examples/bootstrap-catalog.yaml) містить Monitor перед
його Target, CheckProfile, PlacementPolicy та AssessmentPolicy. Посилання
розв'язуються для цілого каталогу, без залежності від порядку документів.

```sh
netmonorc validate -f examples/bootstrap-catalog.yaml -slots branch=office,dc=server
netmonorc-controller -config examples/bootstrap-catalog.yaml \
  -slots branch=office,dc=server \
  -database-file /run/netmonorc/database.dsn \
  -ca /run/netmonorc/ca.crt \
  -cert /run/netmonorc/controller.crt \
  -key /run/netmonorc/controller.key
```

Документаційні IP треба замінити власними дозволеними цілями. Agent і надалі
потребує чинного сертифіката, дозволених CIDR та підготовлених runners.
CLI offline validate не створює призначень і не перевіряє доступність цілі.

## Вибір Monitor та сумісність

За одного Monitor його вибрано автоматично. Якщо Monitor декілька, потрібен
`-monitor NAME`: він визначає основні файли Assessment/diagnostic snapshot,
а всі придатні Monitors виконуються чинним multi-monitor runtime. Вибір першого
документа не використовується як прихований пріоритет. Monitor повинен бути
в цьому пакеті; помилкова або неоднозначна назва відхиляється.

Старі `-diagnostic-profile`, `-alert-rule` та `-webhook-config` працюють далі;
їхні ресурси додаються до тієї самої атомарної операції. Один kind/name не
можна одночасно визначити в пакеті й окремому файлі. Для старих alert/webhook
flags збережено вимогу пари. Усі ресурси, включно з окремими файлами, мають
разом вкладатися в ліміт 20.

DiagnosticProfile, AlertRule та WebhookEndpoint можна розмістити безпосередньо
в пакеті, прибравши відповідні окремі flags. Для доставки webhook потрібні
явні `-webhook-allow-cidrs` і чинні локальні secrets/CA; ці параметри не
виводяться з YAML URL. Наявність allowlist вмикає managed webhook runtime й
без `-webhook-config`. Перевірка локальної конфігурації відбувається до commit.

## Атомарність та повторний запуск

Під спільним із REST apply advisory lock Controller читає збережений каталог,
додає лише ще не створені bootstrap resources, перевіряє повний граф і локальну
webhook configuration. Після цього в одній транзакції зберігає нові ресурси,
immutable source history, bootstrap markers та похідні revisions. Помилка
missing ref, schema, cycle або конфлікт каталогу не залишає частково створені
ресурси чи markers. Сама схема БД може вже бути мігрована через звичайний Open.

| Стан ресурсу | Поведінка при наступному старті |
|---|---|
| Ще ніколи не створювався | Створюється разом з рештою валідного графа |
| Вже є у БД | Збережені manifest/UID/version/generation мають пріоритет над файлом |
| Видалений раніше bootstrap resource | Не відновлюється зі стартового YAML |
| Active Monitor відсутній або видалений | Старт відхиляється без часткового seed |

Зміна або перестановка документів у стартовому файлі не є автоматичним apply.
Для зміни вже створених ресурсів використовується автентифікований CLI/REST
validate → plan → apply з версіями та requestId. Відсутність ресурсу у файлі
не видаляє його з БД. Bootstrap ніколи не очищає PKI, .local або volumes.

## Валідація та межі

CLI та Controller використовують спільний `resources.DecodeBatch`: <=1 MiB на
файл, 1..20 manifests, заборона duplicate keys, YAML merge, alias cycles та
надмірної expansion/depth; збережено numeric types для параметрів CheckProfile.
Невідомі envelope/schema поля відхиляються на відповідному етапі decoder/compiler.
YAML не виконує shell або environment substitution; `${params.*}` має лише
чинну типізовану семантику CheckProfile.

Можна подати один ConfigBatch із manifests замість багатьох документів, але
bootstrap відхиляє непорожні requestId/expectedVersions: умовний apply не
підміняється початковим seed. Каталог загалом обмежений чинними 4096 ресурсами
та 8 MiB. На restart семантично перевіряється фактичний збережений граф плюс
нові ресурси; старі версії spec у стартовому файлі не підміняють чинні з БД.
Пакет не є файловим watcher і не підтягує manifests з мережі.

[Натурні докази](../experiments/bootstrap-catalog-2026-11-20/README.md): missing
ref з нульовим seed, два живі Monitor, shared refs, REST-зміна Target і видалення
невикористаного Target, restart зі зворотним порядком документів без втрати змін.

Для зміни каталогу після старту доступні [YAML-пакети в React та REST](yaml-web.md):
підготовка optimistic versions/requestId, спільна валідація та атомарний apply.
