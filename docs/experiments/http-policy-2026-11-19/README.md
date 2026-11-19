# HTTP policy — натурне приймання

Owned project nmo-http-policy-20261119, приватний HTTPS API, mTLS Agents,
справжні PostgreSQL і мережеві probes. Обидва Agent виконали pinned HTTP redirect
до service.invalid зі збереженим Host і X-Probe; fixture перевіряє їх перед 200.
Assessment став healthy. Redirect на metadata IP дав execution_failed/unknown/
target_policy_denied на обох Agents, Assessment unknown. Початковий Monitor
відновлено до exact revision/healthy, logout виконано. Секретів у live.json немає.

Go та race пройшли, vet без повідомлень. Окремі TLS socket tests перевіряють
SNI/Host, довірений сертифікат і відмову при іншому hostname. Без DSN частина
DB unit tests пропускається; цей live run працював із реальною БД.
Production build включає React. Нової браузерної перевірки не було.
Початкова збірка до виправлення unused import не була прийманням;
збережені final logs належать виправленому коду. runtime.json ідентифікує образ.

```sh
python3 scripts/lab.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_http_policy.py --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python3 scripts/lab.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Перед up зібрати controller/target. Existing users/config/output не перезаписуються.
Стенд зупинено; .local, PKI та volumes збережено. [Контракт і driver limits](../../deployment/http-policy.md).
