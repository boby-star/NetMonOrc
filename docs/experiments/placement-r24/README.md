# Fixed regular PlacementPolicy integration R24

Дата: 2026-11-15 UTC. Runtime image source: af0c84c; CLI workflow flag-order
correction і scope matrix: 18f6019; diagnostic scenario budget: b519c07
(scripts/docs, image code не змінювали).
Project: nmo-placement-r24b, private prefix 172.19, PostgreSQL, HTTPS Controller
і два mTLS Agent. Host ports не публікуються. Image digest у image.txt.

## Перевірений локальний зріз

Критичні tests resources/config/monitor/agent/transport та go vet internal/cmd
пройшли. Actual HTTP scheduler test показав requests від обраного Agent і
нуль від idle Agent. Disposable PostgreSQL/HTTPS workflow пройшов: mixed apply,
JSONB round trip, залежні effective revisions без зміни source version,
delete/If-Match guards і pending operation. Durability/fencing test підтвердив
відмову regular replay з historical idle assignment, відмову forged diagnostic
task і приймання легітимного old regular result. Docker build зібрав усі Go
бінарники; React production build пройшов. DOM clicks не автоматизовані.

## Live workflow

Повний scripts/lab_sessions.py завершився успішно на nmo-placement-r24b;
результат у live.json. Mixed apply дав selected branch і online idle dc;
у поточній ревізії dc не мав regular observations, але виконав окрему native
HTTP diagnostic task із success. Fixed run без hypotheses коректно завершився
inconclusive/fixed_finished_without_class. Після stop branch TTL призвів до
unknown; повернення branch відновило healthy. Policy-only replicas=2 при
одній дозволеній office-зоні дало Selected=[], обидва regular flags=false,
unknown та running operation/insufficient_placement_agents. За два інтервали
не записано жодного regular result pending-ревізії. Restore policy повернув
healthy; source resourceVersion Monitor не змінився. Restart зберіг policy/ref
та desired idle/selected flags. Після unlink/delete відновлено legacy checks
обох Agent, healthy та current heartbeat/revision proofs. Сесія і PAT
відкликані штатними API наприкінці. Попередні critical workflows також пройшли.
Перший stand nmo-placement-r24 завершився timeout перевірочного сценарію:
20s budget не вміщав 20s transport overhead + 5s probe, тому planner правильно
повернув no_admissible_action без physical task. Budget сценарію виправлено
на 40s, runtime code незмінний. Перший stand зупинено зі збереженням volumes;
повторення використовує новий owned project і штатний fresh login.
Сценарій scripts/lab_placement.py перевіряє typed preview, selected branch та
idle dc, diagnostic на dc, втрату branch, pending replicas без partial probes,
policy-only dependent revision і restart. Це fixed-only реалізація: dynamic
strategies, same-zone replicas та multi-Monitor runtime не оголошуються готовими.

Повторення потребує нового owned project без users: lab.py prepare → up →
lab_sessions.py. Після down named volumes зберігаються. Повний scope та
обмеження: IMPLEMENTATION_STATUS.md і docs/deployment/placement-policies.md.

## Доданий React batch preview

Source ac4a832 додав «Показати план пакета» і видимий selected/pending summary
для mixed Monitor + PlacementPolicy. Single-resource та batch preview
використовують спільний HTTP client/server contract; редагування пакета скидає
попередній план. Окремий Docker production build пройшов; digest у ui-image.txt.
Live stand продовжує попередній image af0c84c: Go runtime незмінний. Нову кнопку
не видаємо за перевірену DOM automation. Image digests збережено окремо.

Режим --placement-only додано для наступних вузьких прогонів; цього разу
виконано повний default workflow. Усі JSON exports перевірені на відсутність
PAT, cookie, DSN і private keys; SHA256SUMS фіксує збережені artifacts.
