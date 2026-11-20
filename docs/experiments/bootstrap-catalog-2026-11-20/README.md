# Стартовий YAML-каталог — натурне приймання

Owned project nmo-yaml-catalog-final-20261120, приватні мережі 172.20.x.x,
справжні PostgreSQL/mTLS Agents/HTTPS API та fixture 198.18.0.80. Пакет містить
10 документів: два Monitor, CheckProfile з типізованим URL parameter, Target,
PlacementPolicy, AssessmentPolicy, unused logical Target, DiagnosticProfile,
AlertRule і WebhookEndpoint. Monitors стоять перед своїми refs. Окремі legacy
profile/alert/endpoint flags прибрано; явні webhook CIDR/secrets/CA збережено.

live.json підтверджує:

- broken targetRef у другому Monitor: відхилено повний graph, у PostgreSQL
  рівно 0 resources, 0 source revisions, 0 bootstrap markers;
- після виправлення файла збережено всі 10 ресурсів; branch виконав реальні
  native HTTP checks обох Monitor, dc не виконував чужих primary checks;
- обидві Assessment healthy поточної effective revision, proof IDs реальні;
- REST PUT shared Target criticality=4 змінив залежні revisions обох Monitor,
  не переписуючи їхню source version;
- REST DELETE unused Target успішний; файл далі містить цей документ;
- файл переписано у зворотному порядку, Controller перезапущено: нові за часом
  healthy Assessment обох Monitor мають ті самі revisions, що після REST PUT;
  Target edit і його ETag збережено, unused Target лишився відсутнім;
- Monitor ETags незмінні; 9 чинних ресурсів, 11 immutable source revisions,
  10 bootstrap markers. Restart не додав нової історії; logout успішний.

Первинна спроба на nmo-yaml-catalog-20261120 дійшла до перевірки restart, але
helper помилково очікував REST поле at замість evaluatedAt. Helper виправлено;
повний сценарій пройдено заново на свіжому фінальному проекті, без очищення
даних першого. Обидва стенди зупинено; PKI/.local/volumes збережено.

Після основного trial decoder додатково перевіряє metadata всіх документів,
включно з ресурсами, уже наявними в БД; legacy files використовують той самий
decoder. runtime.json розділяє trial/final image. final-smoke.json підтверджує
свіжі healthy оцінки обох Monitor на фінальному image, збереження criticality=4,
видалення та count=11 історії. Повний broken-ref trial вдруге на final image
не повторювався; metadata paths перевірено unit tests.

Go/vet пройшли. critical.log містить останню перевірку selection/ambiguity,
invalid metadata, duplicate resources і відокремлення apply requestId/версій
від seed. Звичайні DSN-залежні DB tests у go.log пропущено; атомарність і
persistence тут перевірені реальним live сценарієм. Production build включає
React, але UI не змінювався й нових browser clicks не було. Production CLI
перевірив публічний YAML приклад (example-validation.json). Logs нормалізовано
лише trailing spaces; артефакти фіксуються SHA256SUMS.

```sh
python scripts/lab_bootstrap_catalog.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
python scripts/lab_bootstrap_catalog.py verify --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX --output .local/YOUR_NEW_OUTPUT/live.json
python scripts/lab_bootstrap_catalog.py down --project YOUR_NEW_PROJECT --prefix YOUR_PREFIX
```

Helper потребує нового project та незайнятого network prefix, не перезаписує
користувачів/output і не видаляє volumes. Це один Docker host із логічними
зонами, не HA/load benchmark. Жодної публічної мережевої цілі не перевіряли.
[Контракт](../../deployment/bootstrap-catalog.md).
