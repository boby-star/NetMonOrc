# Типізоване regex extraction

Декларативний check підтримує `extract` з DSL: `name`, `from: response.body`,
`using: regex`, `pattern`, `group` (позитивний індекс або ім’я), `as` і `onMissing`.
Defaults: regex, string, unknown. Приклад: `examples/extract.yaml`.
Для extraction runner auto обирає native-http; явний blackbox відхиляється.
Це не новий HTTP probing engine: native runner обмежений GET/HEAD без redirects.

Типи string/integer/float/boolean зберігаються через Agent spool, Protobuf oneof,
PostgreSQL jsonb та CEL. Integer — signed int64 без конвертації через float64;
false і zero є значеннями, а відсутнє поле — відсутнім доказом. У JSON float
записується з десятковою частиною, щоб jsonb не перетворив 1.0 на integer.
Не скінченні floats, overflow і невалідний UTF-8 відхиляються.

`extractionStatus[name]` відокремлений від execution та outcome й містить
статус кожного extractor. Без capture з onMissing=unknown поле відсутнє,
status=missing; з onMissing=error — error.
Помилка конвертації — error. Інші успішно отримані поля зберігаються.
Саме extraction error не перетворює HTTP probe success на failure; стан
ресурсу визначає Assessment/CEL. У прикладі відсутнє ready дає unknown,
а отримане ready=false дає down навіть за HTTP 200/probe success.

`bodyMatchesRegex` — інша семантика: це безпосередня HTTP probe-перевірка.
Невідповідне тіло дає negative probe outcome, у Blackbox використовується
fail_if_body_not_matches_regexp. Порожня відповідь також перевіряється.

Обмеження: 16 extractors на check, pattern до 2048 bytes, нормалізовані
імена CheckID, до 4096 bytes JSON values на observation, bounded response
body та timeout. Raw body не надсилається й не зберігається. RE2 не виконує
довільний код. Значення різних replicas не агрегуються: поточний профіль
дозволяє одного фіксованого Agent на зону.

```bash
go build -o .local/netmonorc ./cmd/netmonorc
go build -o .local/netmonorc-agent ./cmd/netmonorc-agent
go build -o .local/netmonorc-controller ./cmd/netmonorc-controller
python3 scripts/distributed_smoke.py --extract
```

Сценарій виконує справжні HTTP-запити через mTLS-агентів і PostgreSQL,
перевіряє missing/false/zero, точний int64 понад 2^53, float та replay.
Це перевірка інтеграції на одному хості, а не експеримент незалежних зон.
