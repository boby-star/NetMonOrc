# Monitor dependencies shorthand — натурне приймання

Фінальний owned project nmo-shortdeps-final-20261122, приватні HTTPS/mTLS/
PostgreSQL та реальні probes. Вхідний пакет містить Monitor з dependencies:
[yaml-upstream], CheckProfile, його Target та logical yaml-upstream. Жодних
окремих Dependency у БД не створено.

live.json та browser.log підтверджують:

- YAML validate/prepare/plan/apply, Agent ACK та healthy/sufficient із реальною
  Observation; logical dependency без власних probes не підміняє стан Monitor;
- compiled context API має 1 derived requires/soft, from Monitor/yaml-web,
  to Target/yaml-upstream, declaredBy Monitor/yaml-web та configRevision;
- React-кнопка «Контекст Monitor yaml-web» показує цей зв’язок; browser-graph.png
  переглянуто, напрямок і походження видимі. Opaque ID не роздуває кнопку;
- DELETE yaml-upstream відхилено з 409, доки Monitor його перелічує;
- змішаний cycle: явний Target/yaml-upstream → Monitor/yaml-web з implicit
  зворотним ребром відхилено з 422. Інший Target у тому самому batch не
  збережено, його GET дав 404;
- після вилучення списку з Monitor context став порожнім, DELETE Target дав
  204, а фізичний моніторинг повернувся до healthy;
- PostgreSQL Dependency resources=0, Monitor source revisions=2 (create/remove).
  Передані через YAML числові літерали та попередні guards також перевірені.

cli-context.json — справжній netmonorc get dependency-context yaml-web зі
scoped resources:read PAT після вилучення списку: items=[] із поточною compiled
revision. PAT передано stdin, відкликано після перевірки; cookie-сесію завершено.
example-validation.json — production CLI validate публічного YAML прикладу.
CLI не тестував додавання/вилучення shorthand окремо від браузерного apply.

Go/vet і production build пройшли. Unit cases: порядок refs/hash, []/legacy
hash, missing/duplicate/wrong-type refs, змішаний cycle, explicit hard override
без дублювання і derived provenance. Звичайні DSN-залежні DB tests у go.log
пропущено; наведені DB assertions виконав live helper на справжній PostgreSQL.
OpenAPI YAML та всі локальні $ref перевірено; це structural check, не повна
перевірка відповідності всього старого OpenAPI runtime.

Перший успішний trial на nmo-shortdeps-20261122 показав довгий internal ID у
кнопці таблиці. UI підпис виправлено, після чого повний сценарій пройдено заново
на свіжому фінальному project/image. runtime.json фіксує саме його. Обидва
стенди зупинено; .local/PKI/volumes збережено, demo credentials не опубліковано.

Браузер — headless Chromium з тимчасовим профілем і pin лише SPKI owned
Controller; Node окремо перевіряє CA/hostname. Глобальний trust store не змінено.
Це один Docker host із логічними зонами, не HA/load benchmark. Logs нормалізовано
лише trailing spaces. SHA256SUMS фіксує артефакти.

```sh
python scripts/lab_yaml_web.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PRIVATE_PREFIX --port YOUR_FREE_PORT
python scripts/lab_yaml_web.py verify --project YOUR_NEW_PROJECT --prefix YOUR_PRIVATE_PREFIX --shorthand --output .local/YOUR_NEW_OUTPUT/live.json --screenshot .local/YOUR_NEW_OUTPUT/browser.png
python scripts/lab_yaml_web.py down --project YOUR_NEW_PROJECT --prefix YOUR_PRIVATE_PREFIX
```

[Контракт](../../deployment/monitor-dependencies.md).
