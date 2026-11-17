# Завершення пакета діагностичних доказів

Production Controller, PostgreSQL, два mTLS Agent, pinned Blackbox та Chromium
із trusted local CA на новому приватному Compose project. Source/image IDs:
`runtime.json`. Користувачі/паролі попередніх стендів не змінювались.

`evidence.json` — справжній CLI export після внесеної HTTP-відмови:
4 фізичні tasks, 4 Observation, complete=true, session revision і explanation.
Правильність session/task/Agent provenance перевірена. Пароль і PAT не входять
до evidence; тестовий PAT відкликано після використання.

`browser.json` фіксує перевірку React decision journal/confirmation та створення
повного JSON Blob через кнопку пакета. Далі виконуються наявні login/session,
CRUD, graph, дво-Monitor і logout сценарії. `panel.png` — граф демонстраційних
ресурсів; download перевіряється assertions, не цим зображенням.

Цільові Go tests перевіряють точний integer 9007199254740993, явний retention
404, відмову за 403, loop paging, зміну сесії, заборону export running та
неперезапис приватного файла. Production build і vet пройшли. Велика
fault-серія повторно не запускалась; її 15 успішних runs збережено в
[попередньому прийманні](../acceptance-2026-11-17/README.md).

Перша спроба harness з docker cp із tmpfs не зарахована; успішний повтор
читає пакет із приватного /data volume. Наявні .local/PKI/volumes не видалялись.

Відтворення: [export](../../deployment/diagnostic-export.md),
[quickstart](../../deployment/quickstart.md), `scripts/lab_browser.py --multi-monitor`
на новому owned project із новими output paths. Межі точності та complete:
послідовні API reads; complete не означає успішність усіх фізичних перевірок.
