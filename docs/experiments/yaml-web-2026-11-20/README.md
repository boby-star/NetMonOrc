# YAML React/REST — браузерне приймання

Owned project nmo-yaml-web-final-20261120, private HTTPS на 127.0.0.1:8448,
PostgreSQL та mTLS Agents branch/dc. Headless Chromium виконує справжні кліки
й редагування textarea React; direct fetch використано для окремих REST
precondition/CSRF сценаріїв, не замість основного UI workflow.

live.json / browser.log / browser.png підтверджують:

- парольний вхід, YAML із Monitor перед Target/CheckProfile, перевірка valid=true;
- prepare повернув читабельний текст із версіями/null та requestId, але нові
  ресурси залишились відсутніми до apply;
- збережено float 1.0 та integer 9007199254740993 у prepared text, REST і БД;
- plan та apply через кнопки React; фізичний Agent probe дав healthy/sufficient
  для yaml-web із реальною Observation та effective revision;
- повторний клік apply повернув той самий operationId; PostgreSQL має рівно
  одну операцію для цього requestId і одну source revision нового Monitor;
- integer parameter зі значенням 200.0 дав valid=false, а duplicate YAML key — 400;
- нову версію Target застосовано через REST; повторне prepare старого пакета
  не переписало явну стару версію або requestId. Його apply відхилено з 412;
- prepare без CSRF дав 403, після logout без автентифікації — 401;
- браузерних Runtime exceptions немає. Screenshot показує завершений UI apply.

Node HTTPS спочатку перевіряє TLS1.3, локальний CA та hostname Controller.
Chromium має тимчасовий профіль і pin лише публічного SPKI сертифіката цього
Controller. Глобальне сховище довіри та HOME не змінюються. Це не твердження,
що в Chrome встановлено CA, і не перевірка користувацької shared Chrome сесії.

Перші запуски виявили помилки runner: повернення DOM через CDP та передчасне
очікування слова «завершено» у select-фільтрі замість h3 статусу операції.
Runner виправлено; невдалі запуски не прийнято за доказ готового apply. Helper
дозволяє повтор до створення YAML-ресурсів лише зі своїми приватними demo
credentials; користувачів не перезаписує. Фінальний сценарій пройдено повністю.
Обидва owned стенди зупинено, .local/PKI/volumes збережено. Demo credentials
лишилися лише у приватній .local, в артефактах їх немає.

Go/vet і production build пройшли. Звичайні DSN-залежні DB tests у go.log
пропускаються; окремі твердження про PostgreSQL підтверджені live helper.
Unit tests перевіряють стабільний prepared source, effective hash, типи чисел,
явні null/stale versions та відсутність зміни вхідного map; HTTP parser tests
перевіряють MIME, duplicate keys та розмір. Logs нормалізовано лише trailing
spaces. runtime.json фіксує image/base commit; SHA256SUMS фіксує артефакти.

```sh
python scripts/lab_yaml_web.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --port YOUR_FREE_PORT
python scripts/lab_yaml_web.py verify --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json --screenshot .local/YOUR_NEW_OUTPUT/browser.png
python scripts/lab_yaml_web.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Потрібні Chromium/Node та підготовлені images. Це один Docker host, а не
HA/load benchmark. Редактор одного ресурсу лишається JSON; workflow YAML і
збереження початкових числових літералів перевірено в пакетному редакторі.
[Контракт](../../deployment/yaml-web.md).
