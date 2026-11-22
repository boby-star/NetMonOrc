# Аудит YAML/CEL: executable probes

[Основний звіт і матриці](../../specs/yaml-cel-audit-2026-11-22.md).
Reviewed source e2be554d68385c58cdc9bad11afc3902c6b24cfe.

probes.go.txt — точний Go-код read-only перевірок, запущений усередині module
через go run ./.local/yaml-cel-audit. Жодних HTTP mutations, запуску probes
цілей, доступу до секретів або змін runtime source він не робить.
Для відтворення скопіюйте його до main.go в новій локальній піддиректорії
цього репозиторію та виконайте go run для неї: потрібен доступ до internal
packages. Документи прикладів читаються від кореня репозиторію.

probes.json містить 24 cases. accepted=true означає тільки відсутність
compile/decode error, а не «реалізовано правильно». Наприклад, trigger typo
прийнятий, але runtimeError фіксує no such key. Default healthy прийнятий —
це підтверджена розбіжність з DSL, не доказ обходу runtime safety gates.
Kind flags означають Supported(kind), а не повне приймання всіх його полів.

Негативні cases підтверджують відхилення parser duplicates/NaN/alias cycle/
status, невідомих Assessment checks, Fixed refs/fields та відсутніх placement
можливостей. Це не 24 успішні функції: частина cases навмисно документує прогалини.

Поза probes зроблено source review 11 kind, extraction, shared parser,
CEL contexts, placement та operational/declarative boundary.
tests.log — пройдені наявні suites config/resources/monitor/diagnostics/
alerting/probe/CLI. DB-залежні tests за відсутності DSN можуть пропускатися;
нової PostgreSQL/Agent/browser перевірки не було. Не перевірялася кожна
комбінація параметрів усіх drivers. runtime.json фіксує ці межі.
