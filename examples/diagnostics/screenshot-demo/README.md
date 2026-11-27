# Адаптивна діагностика: приклад для рисунка підрозділу 3.1

Окремі нові ресурси `Target/thesis-adaptive-target`,
`DiagnosticProfile/thesis-adaptive-web`, `Monitor/thesis-adaptive-web`.
Існуючі монітори й сервіс NetMonOrc змінювати не потрібно.

Підтверджена прив'язка: `branch=office` на Germany `188.137.224.35`,
`dc=server` на Netherlands `188.137.254.31`. Звичайний монітор виконується
тільки на **branch**. Дві діагностичні дії виконуються на branch, третя —
на **dc**, щоб перевірити HTTP-доступ з іншої зони. Ціль — окремий
демонстраційний HTTP-сервіс на Poland `188.137.244.239:18088`.

## Що саме адаптується

У всіх чотирьох несправностях початкова `/ready` з office негативна.
Ця відповідь не дозволяє встановити причину. Каталог має три додаткові дії:

| Дія | Agent | application-error | backend-error | office-access-restriction | listener-unavailable |
|---|---|---|---|---|---|
| a-office-tcp | branch | success | success | success | failure |
| b-office-backend | branch | success | failure | failure | failure |
| c-server-ready | dc | failure | failure | success | failure |

Перша дія розрізняє три пари гіпотез і має costUnits=0.5: score=6.
Дві інші початково також розрізняють по три пари, але коштують 1: score=3.
Тому спочатку обирається TCP. Після його успіху залишається три гіпотези;
backend і server-ready мають gain=2, score=2. За рівності обирається
`b-office-backend` відповідно до `actionIdAsc`. Після її негативної відповіді
лишаються дві версії, які розрізняє звернення з dc.

```text
Негативна регулярна /ready з office
  └─ a-office-tcp
       ├─ failure → listener-unavailable
       └─ success → b-office-backend
                      ├─ success → application-error
                      └─ failure → c-server-ready
                                     ├─ failure → backend-error
                                     └─ success → office-access-restriction
```

Це три різні довжини сесії: 1, 2 або 3 фізичні запуски. Fixed steps у
маніфесті відсутні: Engine обчислює gain/score повторно після кожної відповіді.
Схема пояснює очікувані рішення, але не задає виконавцю готовий маршрут.
`costUnits` — відносна експертна вартість, не виміряні байти чи секунди.

У цьому навчальному профілі немає окремої політики незалежного confirmation.
Кінцевий reason — `supported_class`, а не `confirmed_class`. Це підтриманий
доказами клас у межах каталогу; невідомі та комбіновані несправності можуть
потребувати ширшої моделі. Зникнення Agent і `execution_failed` не є
автоматичними доказами `listener-unavailable`.

## 1. Запустити окрему ціль на Poland

У папці актуального checkout NetMonOrc відкрийте окрему SSH-сесію:

```bash
python3 examples/diagnostics/screenshot-demo/fixture.py
```

Потрібен Python 3.10+ без сторонніх пакетів. Процес залиште працювати в цій
сесії; для тривалої демонстрації використайте `tmux`. Порт 18088 має бути
вільним і доступним із Germany та Netherlands. Керування fault слухає
**лише 127.0.0.1:18089**. Не публікуйте цей порт і не проксіюйте control API.
Скрипт не змінює firewall, Docker volumes, сертифікати чи сервіси NetMonOrc.
`listener-unavailable` закриває тільки власний сокет fixture; control API
залишається доступним для відновлення. Завершення — Ctrl+C.

HTTP-доступ дозволено тільки з адрес двох стендових вузлів і localhost.
Якщо вихідна адреса змінена NAT/VPN, вкажіть фактичні адреси через
`--office-ip` і `--server-ip`; адреси повинні відрізнятися. Заголовок
X-Forwarded-For не використовується для визначення джерела.

На Poland в іншій SSH-сесії:

```bash
curl --fail http://127.0.0.1:18089/state
curl --fail http://127.0.0.1:18088/ready
```

На Germany та Netherlands виконайте:

```bash
curl --fail --max-time 5 http://188.137.244.239:18088/ready
```

У healthy всі ці запити мають дати 200. Обидва Agents повинні мати
`188.137.244.239/32` у чинному `allow-cidrs`. Перевірте також підтримку
`native-http` та Blackbox TCP в агентах. Якщо потрібне доповнення allowlist,
збережіть уже дозволені CIDR; заміна всього списку одним префіксом може
порушити інші монітори. Старт Python-процесу сам allowlist не змінює.

## 2. Застосувати YAML через адмінку

1. Відкрийте **Конфігурація моніторингу** → **Пакетне застосування й операції**.
2. У поле **Пакет YAML / JSON** вставте весь `manifest.yaml` із цієї папки.
3. Натисніть **Підготувати пакет**, потім **Перевірити пакет** та
   **Показати план пакета**.
4. Переконайтеся, що пакет містить три нові ресурси з наведеними іменами,
   а regular assignment веде на branch/office. Якщо ці ресурси вже існують,
   перегляньте зміни перед повторним застосуванням.
5. Натисніть **Застосувати пакет** та дочекайтеся завершення операції.
6. На сторінці моніторів відкрийте `thesis-adaptive-web` і дочекайтеся
   `healthy` із свіжими даними office. До першої несправності отримайте
   щонайменше два успішні регулярні результати.

Для CLI з актуально зібраним `netmonorc` та вже авторизованим context:

```bash
netmonorc validate --remote -f examples/diagnostics/screenshot-demo/manifest.yaml
netmonorc plan -f examples/diagnostics/screenshot-demo/manifest.yaml
netmonorc apply -f examples/diagnostics/screenshot-demo/manifest.yaml --wait
```

Це remote-команди; за потреби додайте `--context-file /private/context.json`.
Сам факт offline validate не означає застосування на Controller.

## 3. Отримати повну сесію із трьома кроками

На **Poland** переключіть тільки fixture у режим помилки залежності:

```bash
curl --fail -X POST http://127.0.0.1:18089/fault \
  -H 'Content-Type: application/json' \
  -d '{"fault":"backend-error"}'
```

Чекайте нового down монітора й автоматичної сесії (інтервал regular — 10s).
Автоматичний trigger має `onStates: [down]`; за відсутності свіжих даних і
стану unknown він не вигадує відмову цілі. У розділі **Діагностика** виберіть
`thesis-adaptive-web`, оновіть список і відкрийте нову сесію.
Не перемикайте fault до її завершення. Очікувані дані:

| Крок | Agent | Гіпотез перед кроком | Gain | Score | Probe |
|---|---|---:|---:|---:|---|
| a-office-tcp | branch | 4 | 3 | 6 | success |
| b-office-backend | branch | 3 | 2 | 2 | failure |
| c-server-ready | dc | 2 | 1 | 1 | failure |

Підсумок: `completed`, `supported_class`, `backend-error`, 3 attempts,
2.5 costUnits. Час залежить від мережі; точне значення наперед не задається.
Якщо з'явились execution failures, pending або inconclusive, збережіть їх
реальний зміст і перевірте доступність/allowlist/capabilities; не підписуйте
такий кадр як успішну класифікацію.

## 4. Знімки для записки та друга гілка

Зробіть кадр **підсумку сесії** і кадр **журналу рішень** з action ID,
Agent, hypothesesBefore, gain/score та execution/probe результатами.
Рекомендований підпис: «Результати адаптивної діагностики відмови залежності
вебсервісу: а — підсумок сесії; б — послідовність діагностичних дій».
Це рисунок у 3.1 після опису журналу та завершення діагностики.
У тексті назвіть приклад контрольованою демонстрацією, а не вимірюванням
універсальної переваги методу.

Щоб показати, що алгоритм не завжди проходить усі три дії, поверніть healthy:

```bash
curl --fail -X POST http://127.0.0.1:18089/fault \
  -H 'Content-Type: application/json' -d '{"fault":"healthy"}'
```

Дочекайтеся свіжого healthy та завершення попереднього епізоду.
Cooldown — 5 хвилин: якщо наступний автоматичний запуск ще обмежений,
дочекайтеся його завершення; багаторазові кліки запуску не потрібні.
Потім увімкніть іншу несправність:

```bash
curl --fail -X POST http://127.0.0.1:18089/fault \
  -H 'Content-Type: application/json' -d '{"fault":"application-error"}'
```

Очікуються тільки `a-office-tcp → b-office-backend`: обидві відповіді
success, клас application-error, 2 attempts, 1.5 costUnits. Server probe
не потрібен для розрізнення решти гіпотез. Для третьої демонстрації режим
`office-access-restriction` дає 3 кроки, але останній probe success і клас
office-access-restriction. Це реальний HTTP 403 за джерелом, а не емуляція
обриву маршруту. `listener-unavailable` дає 1 крок TCP failure.

Завершіть показ відновленням healthy. Файл маніфесту не містить секретів;
на скриншот не повинні потрапити токени чи приватні ключі.

## Перевірка прикладу

```bash
python3 -m unittest discover -s examples/diagnostics/screenshot-demo -p test_fixture.py -v
go test ./internal/diagnostics -run '^TestScreenshotAdaptiveManifest$' -v
go run ./cmd/netmonorc validate \
  -f examples/diagnostics/screenshot-demo/manifest.yaml \
  --slots branch=office,dc=server --allow-cidrs 188.137.244.239/32
```

Перша перевірка виконує реальні loopback HTTP/TCP-запити та відновлення
listener через control API. Друга використовує реальний Adaptive Engine
і поставлений YAML, перевіряючи чотири класи, призначення агентів, gain/score
й різну довжину маршрутів. Це локальні перевірки; виконання на трьох
віддалених вузлах та фінальний браузерний кадр потребують запуску стенда.
