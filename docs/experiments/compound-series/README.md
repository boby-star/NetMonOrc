# Складені відмови HTTP/TCP/DNS

9 реальних запусків: HTTP+TCP, DNS+TCP і одночасна відмова трьох протоколів
у Static/Fixed/Adaptive. Усі дали down із повним coverage та inconclusive,
без визначеного хибного класу. По одному повтору; source 7bb54b3, дерево
чисте на початку. Фактичні image IDs і порядок запусків є в primary.json.
Це продовження перевірки каталогу, не універсальний доказ для довільних faults.

Adaptive виконав 3 або 4 probes до конфлікту, Fixed 4, Static 6.
При порожній множині сумісних гіпотез не потрібне подальше confirmation.
Показник inconclusive для цих сценаріїв не є точним діагнозом першопричини.
Raw JSON/CSV/log збережено без редагування. RESULTS.md згенеровано програмно.

Відтворення: lab_experiment.py --policy confirmed --repetitions 1
--scenarios http-tcp,dns-tcp,all-protocols з новим output. Ground truth
не передавався планувальнику, стан знімався після свіжих доказів усіх checks.

Обладнання та Docker-хост спільні, трафік у байтах не вимірювався.
