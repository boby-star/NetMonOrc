# Відомі, спільні та невідомі домени відмови

Джерело вимог — §2.1/§2.3 поточної редакції записки, SHA у runtime.json.
Production Controller, PostgreSQL, два mTLS Agent, Blackbox та HTTPS API на
новому приватному Compose project. Одна HTTP-відмова повторена після recovery
у трьох Controller-owned domain configurations з тим самим confirmed profile.

| Mapping Agent → domain | Доменів | Confirmation | Результат | Reserved checks |
|---|---:|---|---|---:|
| branch → office-path, dc → server-path | 2 | satisfied | completed | 4 |
| branch → shared-gateway, dc → shared-gateway | 1 | unsatisfied | inconclusive | 4 |
| Mapping відсутній | 0 | unsatisfied | inconclusive | 4 |

Наявність різних зон не змінилася. Жодного автоматичного domain=zone fallback
немає. Перевірено також поточний Agent API й історичні domains у diagnostic
explanation: дані report та GET detail збігаються. Healthy відновлювався між
епізодами; сесія після останнього сценарію закрита штатним logout.

`live.json` — первинний результат `lab_failure_domains.py`; `runtime.json` —
source commit/image IDs. `go.log` — усі Go packages (окремий PG test DSN тут не
задавався); справжня PostgreSQL інтеграція перевірена live сценарієм. `race.log`
— diagnostics/transport race; vet пройшов для Controller/transport/diagnostics/storage.
Production image включає React build. Нові браузерні кліки цим сценарієм не
перевірялися; UI build не видається за browser acceptance.

Unit regressions покривають invalid/duplicate/unknown Agent mappings, shared та
unknown counting, offline Agent metadata, збереження історичних доменів і
непридатність baseline probe, розпочатого до нового topology epoch.

Це три контрольні епізоди, не статистична оцінка точності. Домени задає оператор
за відомими логічними шляхами; один фізичний Docker-хост залишається спільною
залежністю. Автоматичне вивчення topology і placement distinctBy=failureDomain
не додавалися. Зміна відповідає тільки зазначеному фрагменту записки.

Відтворення на новому owned project без користувачів:

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project YOUR_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_failure_domains.py --project YOUR_PROJECT --prefix YOUR_PREFIX --output .local/NEW_DIRECTORY/live.json
python3 scripts/lab.py down --project YOUR_PROJECT --prefix YOUR_PREFIX
```

Script зберігає test-only command override в приватному domains.compose.json
свого project. Наявні користувачі/output paths відхиляються, secrets не
друкуються. .local, PKI та Docker volumes не видалялися; стенд зупинено зі
збереженням named volumes. [Контракт](../../deployment/failure-domains.md).
