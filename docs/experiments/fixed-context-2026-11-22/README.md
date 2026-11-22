# Fixed CEL: fields, predecessors and deterministic context

Підстава: DSL §11.5/§18/V012, записка §2.3–2.4. [Контракт](../../deployment/fixed-context.md).
Джерело записки: актуальний DOCX SHA256
`910615e8281e4978ffd1b1fe30cafb163b05b14057009e4cc7b41522bd0fd308`.

Фінальний owned project `nmo-fixed-context2-20261122`: production Controller,
Agent, CLI, PostgreSQL, mTLS/HTTPS та реальні HTTP probes із branch і dc.
`live.json` містить такі результати:

- PUT DiagnosticProfile відхиляє невідомі поля task/monitor, невідомий крок
  та self-reference з 422; ETag чинного профілю не змінюється;
- відсутня dependsOn для прочитаного кроку дає valid=false у config/validate
  та 422 у config/plan/apply;
- production CLI приймає публічний YAML fixed-context.yaml і відхиляє помилку
  executionSttaus;
- чотирикроковий DAG виконується з maxParallel=2, закінчується completed,
  має чотири durable tasks і additionalChecks=4. Кінцева умова читає прямого
  та транзитивного попередника; повторний actionRef має власний фізичний task;
- computed key tasks["a-" + "root"] у незалежній b-reader не бачить a-root:
  за maxParallel=1 і 2 обидві сесії inconclusive з одним task a-root, без
  прихованого запуску reader;
- regular monitoring лишається healthy, source ETag Monitor не змінюється.

`go.log`, `vet.log`: go test ./... та go vet ./... пройшли. Unit cases
перевіряють типізовані поля, self/future/missing refs, відсутню залежність,
транзитивні refs, macro shadowing і runtime isolation в обох engines.
Попередні AssessmentRule reference tests пройшли після винесення обходу AST
до спільного celrefs. DSN-залежні unit tests можуть пропускатися; цей live
trial виконав REST/DB assertions на справжній PostgreSQL.
`build.log`: production Docker build. `runtime.json`: образ і base commit.
Паролів, cookies, приватних ключів та DSN у доказах немає.

```sh
python scripts/lab_fixed_context.py prepare --project nmo-fixed-context-new --prefix 172.27
python scripts/lab_fixed_context.py verify --project nmo-fixed-context-new --prefix 172.27 --output .local/fixed-context-new/live.json
python scripts/lab_fixed_context.py down --project nmo-fixed-context-new --prefix 172.27
```

Перша спроба старту зупинилася до API-сценарію: диск заповнився і PostgreSQL
не зміг ініціалізувати pg_wal. Очищено лише невикористаний regular build-cache
Docker; образи, `.local`, PKI та volumes не видалялися. Фінальний запуск
проведено в окремому свіжому project. Обидва стенди зупинено зі збереженням даних.

Межі: один Docker host; нових browser clicks немає. Перевірено саме Fixed
conditions, не нову підтримку values/reasonCode в TaskView, не всі варіанти
відмов, HA або performance. Computed keys не аналізуються статично, але
отримують той самий обмежений predecessor context. 100% готовність ТЗ не заявлено.
