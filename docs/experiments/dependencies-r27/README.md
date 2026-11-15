# Declared Dependency integration R27

Дата: 2026-11-16 (Asia/Jerusalem). Runtime source bb07494, safe confirmation
GET retry in workflow 39c9075 (scripts only). Owned project nmo-dependencies-r27b,
private prefix 172.19. PostgreSQL, HTTPS Controller і два mTLS Agent; host
ports не публікувалися. Digest у image.txt.

Виконано вузький lab_sessions.py --dependencies-only зі штатним bootstrap,
login, cookie/CSRF та PAT. Mixed apply Dependency + logical Target оновив
implicit Monitor revision, дочекався required_agents_applied і нових реальних
probes. Source manifest/resourceVersion Monitor не змінювався.

Reverse requires edge створював цикл: API відхилив його 422, GET підтвердив
404 без часткового запису; desired Monitor revision лишився чинним. Referenced
Target delete повернув 409. Dependency-only soft→hard змінив effective revision
і повторно дочекався Agent ACK; actual web probes лишилися healthy. Logical
Target без власного health не перетворився на failure керованого Monitor.
Це перевірка відсутності вигаданого спадкування стану, не fault injection DNS.

Restart зберіг graph context та hard strength. Dependency deletion прибрав
context, Target став доступний для видалення; початкова effective revision,
healthy та обидва Agent heartbeat/revision proofs відновлені. Немає observations
для додаткових ресурсів: граф не створив власні probes. PAT і cookie штатно
відкликані наприкінці успішного сценарію.

Перший stand nmo-dependencies-r27 дійшов до delete cleanup, але контрольний GET
після прийнятого 204 потрапив у коротке вікно reload Controller і отримав
connection refused. Workflow тепер обмежено повторює тільки safe GET; DELETE
не повторюється. Перший stand зупинено зі збереженням volumes, повний вузький
сценарій повторено на новому owned project. Runtime code не змінювався.

Критичні Go tests: requires/non-requires cycles, self/unknown refs, deterministic
reachable context та defaults. PG/HTTPS: mixed apply/JSONB, context Agent
VerifyPlan, implicit revision без source version change, 428/If-Match, referenced
Target/Monitor delete guard, cycle rollback та видалення context. Go vet
internal/cmd і Docker production build Go/React пройшли; DOM clicks не тестувалися.

Межі: тільки explicit Dependency; shorthand має суперечність DSL §4/§15.1
і не реалізований. Графічний DAG та автоматичний RCA не додаються. Planner
не відсіює гіпотези з graph metadata. Це не повторний повний прогін R1–R26.

Повторення: новий owned project без users → lab.py prepare → up →
lab_sessions.py --dependencies-only; однакові --project/--prefix, новий --output.
Після down named volumes зберігаються. Експорт перевірений на відсутність
credentials; SHA256SUMS фіксує artifacts.
