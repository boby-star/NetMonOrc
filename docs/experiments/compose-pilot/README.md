# Пілот на двох мережевих сегментах

Це **15 натурних запусків по одному повтору**, не статистична оцінка
переваги одного методу. Primary виконано зі source commit `0eb2338`,
чистим деревом на початку та image IDs у metadata. Кожний результат —
реальні Controller/Agent/Blackbox, mTLS, PostgreSQL та різні office/server
bridges одного Docker-хоста. Ground truth задавав окремий harness.

`primary.json` зберігає сирі assessments, diagnostics, tasks/observations і
summary; `primary.csv` — плоскі результати. Методика та повторення:
[research-experiments.md](../../deployment/research-experiments.md).

## Основний прогін

Ресурс має спільний VIP 198.18.0.80. Зональна відмова — явний prohibit route
лише в office namespace; у server namespace маршрут лишається справним.
Resource state семплюється після свіжих observations усіх запланованих
check/source pairs та наступного Assessment. Очікуваний state не є умовою
очікування, тому помилковий стан не може бути «дочеканий» до правильного.

В усіх 15 семплах стан відповів внесеній відмові. У 12 одиничних сценаріях
отримано правильний клас. Для HTTP+DNS Static дав inconclusive, а Fixed
і Adaptive помилково повернули тільки dns-failure: **13 правильних
діагностичних відповідей із 15 у цьому конкретному пілоті**.
Це не оцінка точності для інших мереж, невідомих збоїв чи повторних запусків.

У таблиці: підтверджені completed probes / висновок.

| Сценарій | Static | Fixed | Adaptive |
|---|---|---|---|
| http | 6 / http-failure | 3 / http-failure | 2 / http-failure |
| dns | 6 / dns-failure | 2 / dns-failure | 2 / dns-failure |
| tcp | 6 / tcp-failure | 2 / tcp-failure | 2 / tcp-failure |
| office-path | 6 / office-path-failure | 2 / office-path-failure | 2 / office-path-failure |
| multiple | 6 / inconclusive | 2 / dns-failure | 2 / dns-failure |

Тривалості доступні в CSV/JSON. За одного повтору їх не слід подавати як
узагальнену median/p95 чи доказ прискорення. До duration входять transport,
startup Blackbox, очікування durable ACK/result. Спільний хост і періодичні
regular checks впливають на вимірювання. Трафік у байтах не виміряно.

## Попередній прогін і його межі

`preliminary-route-withdrawal.*` — ще 15 ранніх запусків, збережених для
перевірюваності. Office fault тоді лише видаляв маршрут. Це могло залишити
fallback через default route, тому частина diagnostic tasks завершилася
через безпечний deadline як execution_failed/unknown. Fixed для office-path
дав inconclusive з нулем підтверджених completed probes; reservations не
повернулися в бюджет. Цей недолік не вилучено з даних.

Крім того, ранній sampler очікував потрібний state. Його state_correct
не можна використовувати для оцінювання правильності Assessment. Основний
sampler виправлено: він очікує докази й час оцінки, а не бажану відповідь.
Попередні дані не об'єднуються з основними для обчислення точності або часу.
Їхній source позначено dirty; вони слугують історією інтеграції, а не
відтворюваним основним результатом чистого commit.

## Що залишилося незавершеним

Два хибні висновки в primary вказують на реальну проблему single-fault
каталогу та early stop/Fixed branching. Потрібна підтверджувальна перевірка
інших рівнів або консервативна відмова від одного класу. Результати цього
прогону не є підставою оголосити діагностичний метод завершеним.

Попередній blackhole-подібний сценарій також показав межу clock/deadline
профілю: 5s uncertainty скорочує доступний час diagnostic probe. Потрібна
окрема перевірка мовчазної втрати пакетів і часової моделі, а не лише
моментальної відмови route. Непідтверджений probe не зараховується як failure.

`agent-loss.json` фіксує незалежний контроль: зупинка branch після TTL
дала office=unknown, server=healthy та загальний unknown без нових
діагностичних сесій. Повернення Agent відновило healthy.

`deployment.json` фіксує різні source IP, відсутність host port bindings
і непривілейовані app containers. `fresh-bootstrap.json` підтверджує запуск
іншого project з порожніми volumes та свіжий healthy в обох зонах.

Жодні cert/key/DSN/паролі не входять до цих артефактів. SHA256 сирих файлів
збережено в sha256.json. Стенди після перевірки зупинено; дані лишаються
у private named volumes та експортованих артефактах.
