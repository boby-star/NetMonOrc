# Запуск робочого дослідницького профілю

Потрібні Linux Docker Engine із Compose v2, Python ≥3.10 та OpenSSL.
Go/Node на хості для запуску не потрібні: вони використовуються у Docker build.
Стенд містить Controller, PostgreSQL, два mTLS Agent, мережеві сегменти та
керовану тестову ціль. Це локальний дослідницький профіль, не повний v1alpha1.

Для нового project (наявні volumes/користувачів не перезаписуйте):

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project netmonorc-demo
python3 scripts/lab.py up --project netmonorc-demo --ui-port 8443
python3 scripts/lab.py admin --project netmonorc-demo --username admin
```

Остання команда запитає пароль приховано у терміналі: 16–256 UTF-8 bytes.
Пароль не передається в аргументах або env. Для автоматизації є
`--password-stdin`: передавайте його захищеним pipe. Початковий admin
створюється тільки за відсутності користувачів; повторний запуск не змінює
паролі й не запитує їх знову. Для подальших користувачів використовуйте Users
у панелі. Це не обхід входу і не команда скидання адміністратора.

Відкрийте **https://localhost:8443** після додавання публічного локального CA
`.local/lab/netmonorc-demo/pki/ca.crt` до довірених сертифікатів вашого браузера.
Не імпортуйте приватний ca.key і не вимикайте TLS verification. Дослідницькі
сертифікати чинні дві доби; enrollment/rotation для production ще відкриті.
На віддаленому Docker daemon localhost стосується його машини: користуйтесь
локальним port forwarding, а не зміною bind на 0.0.0.0.

Явний --ui-port публікує тільки Controller HTTPS на 127.0.0.1. Для цього
Controller отримує окремий UI bridge; PostgreSQL/Agent/Exporter не підключені
до нього та не публікуються. Без прапорця новий project лишається повністю
без host ports, як у попередніх сценаріях. Обраний порт зберігається в
приватному ui.compose.json і використовується наступними командами цього
project. Зайнятий порт призводить до помилки Docker, дані не скидаються.

## Перезапуск і збереження даних

```sh
python3 scripts/lab.py status --project netmonorc-demo
python3 scripts/lab.py down --project netmonorc-demo
python3 scripts/lab.py up --project netmonorc-demo
```

Down зберігає named volumes та конфігурацію. Prepare потрібний для нового
стенда, а не для кожного рестарту. Для зміни/відключення порту спершу down,
потім up із новим --ui-port; значення 0 прибирає host publishing. Не запускайте
prepare поверх живого стенда. Backup містить приватні дані й зберігається
локально з mode 0600:

```sh
python3 scripts/lab.py backup --project netmonorc-demo --output .local/demo-backup.dump
```

[Відновлення](backup-recovery.md), [контрольовані відмови](research-compose.md),
[інтерфейси](../specs/thesis-scope-2026-11-13.md). За корпоративного TLS proxy
використовуйте наявний build-CA override з research-compose.md, з перевіркою TLS.

## Що можна демонструвати

Реальні probes із двох сегментів, state/coverage, декларативні ресурси й
залежні ревізії, CEL/extraction, Static/Fixed/Adaptive діагностику,
alerts/webhook, users/PAT, історію, ResourceGroup та граф Dependency.
Controller виконує [кілька Monitor із каталогу](multiple-monitors.md);
у верхньому полі панелі можна перемикати перегляд без нового входу.
Dynamic placement, replica voting та Agent enrollment залишаються відкритими. Фактичний залишок —
[readiness.md](../development/readiness.md).

## Перевірка браузером проти справжнього Controller

На окремому новому owned project увімкніть --ui-port, потім запустіть:

```sh
python3 scripts/lab_browser.py --project YOUR_FRESH_PROJECT --prefix YOUR_PREFIX \
  --output .local/new-browser-result.json --screenshot .local/new-browser-panel.png
```

Потрібні Node 24+, Chromium та certutil. Script відмовляється працювати поверх
наявних users/output files, створює початкового test admin через звичайну
команду admin, перевіряє її idempotency та реальні browser flows.
Пароль існує лише в пам'яті/stdin, не друкується. Тимчасовий browser home/profile
має власний NSS trust store із публічним test CA; користувацький trust store
не змінюється, TLS checks не вимикаються. Chromium без sandbox використовується
лише як локальний headless acceptance client. Після успішного запуску test
resources видаляються UI, сесія закривається logout; data volumes лишаються.

Додайте `--multi-monitor` до lab_browser.py для перевірки двох одночасних
Monitor, різних результатів та перемикання UI зі збереженням вибору після reload.
