# YAML controls та канонічні ревізії — 18.11.2026

## Реалізована поведінка

Канонічні DiagnosticProfile hashes; offline multi-document/ref validation через
Controller registry; inline target context; Monitor/check enabled;
між-Monitor regular priority і відтворюваний bounded jitter;
weighted.enabled=false/defaultState з evidence guards; maxNoGainSteps=0..5;
відхилення duplicate JSON keys до typed decode. [Контракт і залишок](../../deployment/yaml-controls.md).

## Натурний сценарій

`live.json`: свіжий власний project nmo-yaml-final-20261118, mTLS Agent,
реальні Blackbox probes, PostgreSQL і приватний HTTPS REST:

1. Зворотний порядок DiagnosticProfile.actions зберіг profile hash і залежну
   Monitor revision.
2. HTTP priority=90/jitter=1s, TCP priority=10, DNS enabled=false з узгодженим
   assessment.checks дали healthy з complete coverage у двох зонах. У новій
   ревізії зафіксовано лише HTTP/TCP, DNS observations=0.
3. Виробничий CLI validate прочитав YAML/JSON multi-document комплект зі stdin,
   перевірив refs і повернув offline/valid/planningRequired з двома ресурсами.
4. Monitor enabled=false дав unknown/monitor_disabled та resource status=disabled,
   не створив regular observations і відхилив новий manual diagnostic (409).
5. Відновлено початкову конфігурацію й точну healthy revision; logout пройшов.

`upgrade.json`: на тому самому власному fixture зупинено Controller, похідний
profile hash зроблено застарілим, запущено фінальний Controller image.
LoadRuntime відновив канонічний hash; source manifest digest, generation і
resourceVersion залишилися незмінними. Отримано щонайменше шість свіжих
regular Observation та healthy після рестарту. Це перевірка міграції похідного
hash, а не інсталяційний/HA тест.

`runtime.json` розрізняє image основного acceptance і фінальний image для
upgrade-перевірки: між ними додано тільки refresh похідного profile hash.
Інший production код не змінювався. Збережено base commit і fingerprints
фінальної реалізації. Обидва images побудовано до коміту.

Перший натурний attempt не зараховано повністю: функціональні assertions
пройшли, але відновлення профілю потрапило у reload після відновлення Monitor.
Додано очікування exact healthy revision перед наступним PUT і повторено
весь сценарій на іншому свіжому project. Жодні попередні volumes/PKI не видалено.

## Перевірки компілятора та runtime

`go.log` — усі Go packages; `race.log` — agent/config/resources/diagnostics/httpapi;
`vet*.log` — vet усіх пакетів і фінальної storage зміни. PostgreSQL test DSN
окремо не задавався; реальна БД перевірена натурними сценаріями. Production
image містить успішну React build; нових browser clicks не було.

Цільові регресії перевіряють збереження default revision, disabled coverage,
priority через реальний HTTP Agent scheduler, jitter determinism/bounds,
required/missing guards для unweighted healthy, canonical collection order
без зміни джерела, no-gain ledger без поповнення та duplicate YAML/JSON keys.

`compiler-audit.json` містить повторні 35 проб із попереднього аудиту. Це не
100% conformance всього DSL. Зокрема, нормативний приклад §23.1 тепер проходить
registry compile, priority/jitter/unweighted/maxNoGainSteps приймаються,
перестановка adaptive actions зберігає hash. Check.enabled=false для required
http без зміни явного assessment.checks відхиляється вже через несумісні
вимоги coverage, а не через невідоме поле. Це правильне відхилення.

Audit generator збережено як compiler_audit.go.txt; для запуску скопіюйте його
в новий каталог під .local репозиторію і виконайте go run цього каталогу.
Проби не надсилають мережевих запитів до цілей і не змінюють source код.

## Повторення

```sh
docker compose build controller target
python3 scripts/lab.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab.py up --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python3 scripts/lab_yaml_controls.py --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python3 scripts/lab_yaml_controls.py --upgrade-only --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/upgrade.json
python3 scripts/lab.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

За потреби власного build CA використовується deployment/build-ca.compose.yaml.
Основний сценарій вимагає project без користувачів, upgrade-only — спеціальний
fixture з єдиним yaml-admin. Output не перезаписується. Стенди зупинено,
.local, PKI та named volumes збережено. Logs нормалізовано тільки видаленням
кінцевих пробілів рядків.
