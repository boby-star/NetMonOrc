# Мовчазна втрата DNS-відповіді

Source commit: `5d123e3`, clean tree на початку; image IDs і policy є в
primary.json. Три запуски — по одному для Static, Fixed і Adaptive.
Fixture приймає UDP query, але не надсилає DNS response. Це не NXDOMAIN
і не підставлений execution/probe outcome. Root target лишається HTTP/TCP
доступним, а регулярний DNS check має timeout 2s, diagnostic — 10s.

В усіх трьох випадках отримано down із full coverage та клас dns-failure.
Кожна з двох diagnostic DNS перевірок фактично тривала щонайменше 9.5s
за timestamps (налаштовано 10s) і завершилася completed/failure. Жодного
execution_failed у діагностичних observations не було. Це підтверджує,
що транспорт/clock margin більше не обрізає probe до приблизно 5s.

| Режим | Completed probes | Час Engine.Run у цьому запуску |
|---|---:|---:|
| Adaptive confirmed | 4 | 23611 ms |
| Static | 6 | 24551 ms |
| Fixed confirmed | 4 | 23433 ms |

Це спостереження одного повтору, не доказ переваги за latency. Час включає
orchestration/ACK/startup. Набір не об'єднується зі швидким NXDOMAIN у
статистиці. Timestamp перевірка використовує синхронізований спільний
стенд; Engine duration вимірюється монотонним годинником.

Відтворення: `lab_experiment.py --policy confirmed --scenarios dns-drop`
з новим output. [ADR 0002](../../adr/0002-probe-time-and-task-budget.md)
описує запас часу всередині незмінного global budget, поновлення lease
перед task і preflight Agent. Scope live one-shot — 100ms..15s; довгі regular
checks, OS suspend/BOOTTIME, queue/backpressure та довші one-shot задачи
потребують окремої стабілізації. Це не реалізація всіх випадків packet loss.
