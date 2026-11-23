# Зональні AlertRule, selector та CEL: 2026-11-23

Артефакти цієї ітерації стосуються декларативних сповіщень у мінімальному
дослідницькому прототипі; це не сертифікація повного старого ТЗ. Контракт:
[zone-alerts.md](../../deployment/zone-alerts.md). Базовий commit, production image,
SHA-256 змінених source files і записки наведені в runtime.json.

## Реальний запуск

Наскрізний запуск завершився успішно: live.json має status=passed. Перевірка використовує власні PostgreSQL,
Controller, двох mTLS Agents, native HTTP probe, несправність маршруту office
та HTTPS webhook. Її критерії:

- apply завершується після оцінювання rule hash на обох required zones;
  порожній selector має окремий reason alert_selector_empty;
- selector не створює екземплярів для невибраного Monitor;
- office і server мають різні alert ID; firing office не змінює healthy server;
- firing evidence походять лише з office; ACK одного екземпляра не впливає на інший;
- активна прив'язка/DELETE правила та DELETE Monitor із selector-based firing
  відхиляються з 409; помилкове типізоване CEL поле відхиляється з 422;
- новий збіг selector починає оцінюватися; вилучений Monitor залишається firing
  до справжнього recovery, а потім не починає новий цикл;
- ACK зберігається під час recovery і очищається на новому firing cycle;
- шість логічних webhook events доставлено, всі мають zone=office;
- регулярний моніторинг повертається до healthy; resolved правило видаляється.

```sh
python scripts/lab_zone_alerts.py prepare --project <new-owned-project> --prefix <unused-prefix>
python scripts/lab_zone_alerts.py verify --project <new-owned-project> --prefix <unused-prefix> --output .local/<new-result>.json
python scripts/lab_zone_alerts.py down --project <new-owned-project> --prefix <unused-prefix>
```

Helper відмовляється використовувати вже ініціалізовані облікові записи або
перезаписувати результат. У статусі in_progress запуск не вважається успішним.
Down зберігає .local, PKI та Docker volumes. Паролі, cookies, CSRF і private keys
не експортуються до доказів.

Під час розробки виправлено повторні DDL-міграції адміністративного bootstrap,
які могли deadlock з працюючим Controller. OpenAdmin повторно використовує
наявну схему, а нову БД ініціалізує звичайним шляхом. Helper також дочікується
після reload і трактує тимчасову відсутність assessment як очікування. Загальний
стан при down office/healthy server може бути degraded; перевіряється саме zone.

## Інші перевірки та межі

Go test ./... і go vet ./...: журнали go-test.log/go-vet.log. Тести Go, які
потребують NETMONORC_TEST_DSN_FILE, у цьому виклику пропущені; PostgreSQL/API
сценарії натомість перевіряє наведений реальний стенд. Окремий regression test
перевіряє статичні zone.checks та zones.*.checks refs. CLI production binary
валідує повний examples/zone-alerts.yaml: example-validation.json.
Production Docker build включає React/TypeScript/Vite: production-build.log.

Нових browser interactions немає. Один Docker host не доводить поведінку
різних фізичних fault domains. Reload Monitor тимчасово перепіднімає HTTPS API;
нульовий downtime тут не підтверджений. Повторне створення rule UID, вилучення
required zones при активному alert та повний обсяг старого ТЗ не закриті цим
сценарієм. SHA256SUMS захищає цілісність пакета, а не засвідчує ширший обсяг.
