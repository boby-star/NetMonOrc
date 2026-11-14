# Керування DiagnosticProfile (R11)

Каталог PostgreSQL підтримує Monitor та DiagnosticProfile через наявні
`/api/v1/resources/{kind}`: create/list/get/replace/delete, status і revisions.
Одиночні зміни використовують cookie+CSRF або scoped PAT, ETag/If-Match;
історія, audit, generation та resourceVersion мають ті самі правила, що Monitor.
Назва унікальна в межах kind: Monitor/example і DiagnosticProfile/example
мають різні UID. Status `stored` означає валідований запис каталогу,
не виконану діагностику чи healthy.

React-редактор має вибір Monitor/DiagnosticProfile та каталог із пагінацією;
відкрийте ресурс зі списку або введіть назву. Для створення нового скопіюйте відповідний маніфест,
змініть metadata.name і натисніть «Створити новий ресурс». JSON і server-side
валідація спільні для обох kind. CLI підтримує:

```bash
netmonorc get --context-file /private/context.json DiagnosticProfile
netmonorc get --context-file /private/context.json DiagnosticProfile web-lab
netmonorc plan --context-file /private/context.json -f batch.yaml
netmonorc apply --context-file /private/context.json -f batch.yaml --wait
```

## Узгоджене оновлення залежностей

`config/validate`, `plan` та `apply` розглядають об’єднаний каталог БД і пакета.
Профіль можна створити разом із Monitor у довільному порядку документів.
Перед записом повний граф повторно перевіряється в транзакції під тим самим
блокуванням, що й усі resource mutations. Помилка відкочує весь пакет.

Зміна профілю змінює effective revision кожного Monitor з відповідним
profileRef. Власні manifest/resourceVersion/generation Monitor залишаються
незмінними. Це зміна залежності, а не непомітний PUT Monitor. Нормалізований
профіль входить у hash плану; labels/annotations профілю в hash не входять.
У `plan.diffs` залежний Monitor має action `reconcile`, а в Operation targets —
`implicit: true`. Profile-only apply не завершується до підтвердження нової
ревізії потрібними Agent. Невибраний Monitor залишається pending, як раніше.
Профіль без залежних Monitor завершує застосування після каталогізації.

Видалення профілю з enabled=true посиланням заборонено (409). Вимкнене
посилання може залишатися без профілю, що зберігає попередню поведінку;
увімкнення такого binding без доступного профілю відхиляється (422).
Якщо вимкнене посилання має доступний профіль, його hash також входить
у effective revision, але сам факт наявності профілю не запускає діагностику.

## Bootstrap і перезапуск

`--diagnostic-profile` тепер імпортує початковий ресурс у БД лише один раз.
Наступні старти використовують збережений ресурс; файл не скасовує PUT/apply.
Збережений marker видалення не дозволяє старому bootstrap-файлу воскресити
видалений профіль. Для повторного створення використайте явний POST/apply.
Активний Monitor і профілі компілюються зі спільного транзакційного snapshot.
Watcher чинного Monitor виконує наявне graceful reconciliation; давні
observations не стають поточними доказами нової ревізії.

Залишено обмеження дослідницького runtime: один активний Monitor, fixed roster,
діагностичний timeout до 15s, без непідтверджених byte budgets. Каталог
обмежено 4096 ресурсами та 8 MiB сукупного JSON; пакет — 20 ресурсами/1 MiB.
Це обмеження реалізації, а не нові DSL поля. Довільні kind, multi-Monitor,
динамічний placement і публічний manual start цим етапом не реалізовані.

## Перевірка

Критичний PostgreSQL/HTTPS сценарій перевіряє mixed apply з однаковими
іменами kind, залежні hashes та незмінність власної версії Monitor,
stale If-Match, delete guard, missing profile, restart compilation і marker.
Compose-сценарій змінює costUnits профілю через CLI apply --wait, очікує
справжні Agent ACK, перевіряє поточний Assessment, перезапускає Controller
і відновлює профіль через apply. Алгоритми Gain/Cost, confirmation і budgets
не змінювалися. Докази: [profile-resources-r11](../experiments/profile-resources-r11).
Browser DOM взаємодії не автоматизовано; React перевірено production build.
