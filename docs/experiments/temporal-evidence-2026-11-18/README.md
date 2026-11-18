# TTL та maxSkew у діагностичному висновку

Підстава: §2.1–2.3 записки, source SHA та image ID у `runtime.json`.
На новому приватному стенді працювали PostgreSQL, Controller і два mTLS Agent.
Fixed profile на dc виконував два native HTTP probes: / і /slow (12 секунд).
Обидва фізичні результати — completed/success; перший розрізняє модельні
class-a/class-b, другий сам по собі не визначає один клас. Це контроль
часової придатності, а не експеримент із пошуку фізичної першопричини.

| Monitor freshness | Результат | Фінальні гіпотези | Spent checks |
|---|---|---|---:|
| maxAge=20s, maxSkew=15s | completed | class-a | 2 |
| maxAge=20s, maxSkew=5s | inconclusive | class-a, class-b | 2 |
| maxAge=10s, maxSkew=15s | inconclusive | class-a, class-b | 2 |

`live.json` зберігає policy та рішення з observationId/finishedAt/validUntil.
Для кожного рішення фізичний finishedAt звірено з точним GET Observation:
час приймання відповіді не підставлявся. Execution/probe outcomes не
перейменовано на failure через непридатність доказу. Budget не повертався.
Початковий Monitor відновлено, дочекалися саме його ревізії; logout пройшов.

`go.log` — усі Go packages (окремий новий PostgreSQL test DSN не задавався),
`race.log` — diagnostics/transport. Vet пройшов для змінених runtime packages.
Реальна PostgreSQL/HTTPS інтеграція перевірена цим live сценарієм. Production
image включає React build; нові browser clicks у цій ітерації не проводились.
Unit regressions перевіряють межу maxSkew, TTL, delayed/future timestamps,
latest-pair без повернення зручного старого результату, physical spacing,
baseline + новий результат поза вікном та завершення часткового technical
retry без відновлення старого proof або бюджету.

Перший live attempt не зарахований повністю: усі case assertions пройшли,
але logout потрапив у reload під час відновлення конфігурації. Після
виправлення revision-specific wait повний сценарій повторено на іншому
новому project; тут наведений тільки його успішний результат.

Відтворення на новому owned project без користувачів:

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project YOUR_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_temporal_evidence.py --project YOUR_PROJECT --prefix YOUR_PREFIX --output .local/NEW_DIRECTORY/live.json
python3 scripts/lab.py down --project YOUR_PROJECT --prefix YOUR_PREFIX
```

Результат не є статистичною гарантією, NTP-калібруванням чи доказом фізичної
одночасності різних хостів. [Алгоритм і межі](../../architecture/diagnostic-evidence-window.md).
.local, PKI та Docker volumes не видалялися; стенди зупинено зі збереженням даних.
