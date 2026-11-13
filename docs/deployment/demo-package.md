# Демонстраційний пакет без runtime-секретів

Після підготовки й запуску власного стенда:

```bash
python3 scripts/lab.py package --project netmonorc-lab --output .local/netmonorc-demo.tar.gz
```

Застосунок використовує лише дозволені імена файлів: assessment.json,
report.html та наявні diagnostic.json, alert.json, delivery.json.
README.txt і SHA256SUMS додаються автоматично. Файли конфігурації, приватні
ключі, токени, PostgreSQL DSN та Agent spool до архіву не входять. Звіти
містять назви ресурсів, зон і ID доказів, тому архів приватний: mode 0600.
Попередній архів не перезаписується — для нового знімка задайте новий шлях.

Це послідовні snapshots файлів, а не єдина транзакція. Час і ревізії слід
читати всередині кожного файла. Старі діагнози не є актуальним Assessment,
а delivery summary не оновлюється після копіювання. Зупинений через down
стенд не має контейнера для читання snapshots: створіть пакет до down.

## Перегляд

Розпакуйте власний архів у новий приватний каталог і перевірте SHA256SUMS:

```bash
mkdir -m 700 demo-view
tar -xzf .local/netmonorc-demo.tar.gz -C demo-view
cd demo-view
sha256sum -c SHA256SUMS
```

Відкрийте report.html локально: сервер, інтернет і Go для цього не потрібні.
Попередження про вік Assessment з'явиться, коли snapshot застаріє.

## Повторне формування HTML

За наявності Go ≥1.25 зберіть CLI з тієї самої версії репозиторію:

```bash
go build -o .local/netmonorc-report ./cmd/netmonorc-report
.local/netmonorc-report -assessment demo-view/assessment.json -diagnostic demo-view/diagnostic.json -alert demo-view/alert.json -delivery demo-view/delivery.json -output demo-view/rebuilt.html
```

Прапорці diagnostic/alert/delivery необов'язкові. Передайте їх лише для
файлів, які є в архіві. CLI відхиляє невідомі JSON-поля та delivery/diagnostic
іншого Monitor. Поля delivery відображають snapshot PostgreSQL, не виконують
повторних HTTP запитів і не змінюють стан алертів.

Snapshot доставки задається Controller прапорцем `-delivery-file`;
Compose вже налаштовано на `/data/delivery.json`. Нових REST routes чи
YAML-полів немає. Для старого стенда потрібне оновлення image і контейнера.

Пакет не замінює [backup PostgreSQL](backup-recovery.md), конфігурацію
інсталяції або вихідний код. Код і результати експериментів зберігаються
окремо в GitHub-гілці prototype/minimal-research.
