# ResourceGroup integration R26

Дата: 2026-11-16 (Asia/Jerusalem). Runtime source: c72d6a5. Owned project
nmo-groups-r26, private prefix 172.19. PostgreSQL, HTTPS Controller і два mTLS
Agent; host ports не публікувалися. Digest у image.txt.

Виконано lab_sessions.py --groups-only зі штатним fresh bootstrap/login,
cookie/CSRF і PAT. Mixed apply створив чотири групи на основі реальних
HTTP/TCP/DNS observations активного Monitor та standby без виконання probes.
All-required повернув unknown з healthy active та unknown standby, coverage
degraded. Weighted із .95/.05 та explicit minCoverage=.95 дав healthy,
зберіг degraded coverage і unknown member. Context default дав state=null.

Selector any із minCoverage=1 спочатку unknown. Зміна labels standby виключила
його зі складу: стан healthy, implicit ResourceGroup target в operation,
новий desired revision групи без зміни source resourceVersion. Нуль
observations для імен груп і standby підтвердив відсутність нових probes.
Restart зберіг membership/конфігурацію; після перевірки групи й standby видалено,
original labels відновлено, heartbeat/revision proofs підтверджено. PAT і cookie
відкликані через API, private CLI context видалено logout.

Критичні pure tests: all/any/weighted, unknown/stale/future, default weighted
threshold=1 і explicit .9, zero coverage, context та selectors/missing keys.
Реальний PG/HTTPS: JSONB round trip, old revision fencing, explicit member
delete guard, 428/If-Match, weighted score .95→degraded за default і healthy
за explicit .9. Go vet internal/cmd та Docker production build усіх Go binary
і React пройшли. DOM clicks не виконувались.

DSL healthyMin default виправлено на 1.0 на підставі раніше погодженого Q04;
OpenAPI уточнений typed DTO без нового маршруту. Межі: 100 members, повна
explicit карта weights, on-demand assessment. Multi-Monitor execution та
Dependency не оголошуються готовими; цей вузький запуск не повторює R1–R25.

Повторення: новий owned project без users → lab.py prepare → up →
lab_sessions.py --groups-only. Однакові --project/--prefix, новий --output.
Після down named volumes зберігаються. JSON перевірений на відсутність
credentials; artifacts зафіксовані SHA256SUMS.
