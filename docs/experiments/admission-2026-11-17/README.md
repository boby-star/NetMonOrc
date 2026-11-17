# Спільний Agent для двох діагностик — §2.3 записки

На новому приватному Compose project запущені Controller, PostgreSQL, два
mTLS Agent та керована ціль. Два Monitor одночасно вручну запустили однаковий
Fixed profile із native HTTP /slow (12 секунд), явно призначений Agent dc.
Профіль має maxAdditionalChecks=1, maxDuration=60s, maxParallel=1.

Факти (`live.json`):

- Обидві сесії були running; використаний budget у цей момент: 0 та 1.
- Обидва фізичні probes завершилися completed/success на dc.
- Другий probe почався після завершення першого; busy-відмов немає.
- Загальний used_checks: 2, по одній спробі кожного Monitor.
- Під час першого 12-секундного probe dc передав 6 regular observations.

У Fixed-профілі навмисно немає каталогу гіпотез: перевіряється місткість,
а не точність класу несправності. Результати не доводять FIFO fairness,
HA, масштабування чи незалежності фізичних зон. Blackbox shutdown-path
перевірений кодом/unit suites; ці два live probes використовують native HTTP.

`go.log` — усі Go packages (без окремого нового PostgreSQL test DSN),
`race.log` — targeted race agent/diagnostics/transport. Vet пройшов для змінених
packages та coordinator. Справжня PostgreSQL інтеграція перевірена саме live
сценарієм. `runtime.json` містить base SHA та image ID; образ зібрано з
робочими змінами перед їх комітом. Секрети/PKI/DSN не входять до evidence.

Відтворення на новому owned project, не поверх наявних користувачів:

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project YOUR_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_admission.py --project YOUR_PROJECT --prefix YOUR_PREFIX --output .local/NEW_DIRECTORY/live.json
python3 scripts/lab.py down --project YOUR_PROJECT --prefix YOUR_PREFIX
```

Стоп зберігає named volumes. У цій ітерації не видалялися існуючі файли
.local, PKI чи Docker volumes. [Алгоритм](../../architecture/diagnostic-admission.md).
