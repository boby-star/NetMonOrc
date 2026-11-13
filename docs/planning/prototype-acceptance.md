# Приймання мінімального дослідницького прототипу

Ця таблиця зіставляє вимоги записки (§1.4, §2.1–2.2) з фактичною програмою.
Повний продукт v1alpha1 і оцінювання роботи кафедрою не охоплені.

| Потрібний результат | Реалізація | Свідчення / межа |
|---|---|---|
| Monitoring as Code | internal/config, Monitor і DiagnosticProfile YAML | Closed decoder; непідтримувані refs/kinds відхиляються |
| Дві точки спостереження | Controller, branch, dc; різні office/server маршрути | Compose pilots, зональний prohibit route; обладнання спільне |
| Стандартні probes | Blackbox 0.27.0 HTTP/TCP/DNS/ICMP | Network smoke, ICMP ізольований smoke; ICMP не включено в основний Compose |
| Прикладна перевірка | native HTTP extraction → typed values → CEL | Distributed extraction smoke; raw body не зберігається |
| Комплексний стан | H/Q, required, TTL/maxSkew, revision, zonal all-required | Assessment unit/integration та live fault scenarios |
| Достовірність походження | mTLS, zone roster, generation, ACK після commit | PG fencing, replay/conflict tests; preissued PKI |
| Обмежена діагностика | Static/Fixed/Adaptive, Gain/Cost, eligibility, budgets | Live pilots; одна сесія на інцидент, serial execution |
| Уникнення вигаданої причини | confirmation, inconclusive, розділення execution/probe | HTTP+DNS counterexample і виправлений confirmed pilot |
| Сповіщення | AlertRule FSM, один HTTPS webhook, durable outbox | Webhook pilot з 503/retry/restart, окремі firing/resolved |
| Перегляд результатів | Приватний read-only HTML, CLI та JSON | Експорт звіту без публічного listener; це не React CRUD UI |
| Експериментальні дані | JSON/CSV, повторювані fault controls, Markdown report | scripts/lab_experiment.py та experiment_report.py |
| Збереження роботи | Окрема GitHub branch, pg_dump, документація restore | Restore у тимчасову БД реально перевірено |

## Межі, які потрібно перенести до записки

- Поточний профіль має один fixed Agent на зону. Multi-replica all/any/quorum
  не реалізовані як окрема функція; при N=1 політики збігаються. Не писати,
  що різні quorum-пороги були перевірені цим прототипом.
- ResourceGroup та довільний Dependency graph — моделі можливого розширення,
  не реалізовані експериментально підтверджені компоненти цієї версії.
- Зони різні логічно; фізичний хост, CPU та Docker daemon спільні.
- Не називати Gain/Cost і H/Q ймовірностями; costUnits не є байтами.
- Множинна/невідома несправність може мати той самий pattern, що відома.
  Confirmation прибирає конкретний контрприклад, а не всі можливі помилки.
- Linux BOOTTIME перевіряється перед новими probes; фактичний OS suspend
  у Cloud не проводився. Рестарт потребує нового lease, saved lease не resume.
- Спостереження, отримані до відмови джерела, діють до TTL. Offline Agent не
  дорівнює негайному down сервісу. HTML snapshot теж може застаріти.
- Немає повного REST/RBAC/enrollment/HA/динамічного placement/hostname policy.
  Ці функції не слід показувати в роботі як реалізовану продуктову платформу.

## Відтворення демонстрації

1. Зібрати pinned images та підготувати приватний Compose stand за
   docs/deployment/research-compose.md.
2. Запустити up; дочекатися нових healthy спостережень двох Agent.
3. Внести http/dns/tcp/office-path/multiple, перевірити Assessment/diagnostic
   та відновити healthy між епізодами.
4. Провести repeated series однакових каталогів/бюджетів, зберегти JSON/CSV
   та згенерувати описовий звіт через scripts/experiment_report.py.
5. Експортувати HTML і зробити приватний backup. down зберігає volumes.

Остаточна готовність для записки потребує внесення результатів і цих меж
до її фінальної версії. Ця матриця не змінює текст завантаженого DOCX
і не маскує непідтримувані моделі як завершені функції.
