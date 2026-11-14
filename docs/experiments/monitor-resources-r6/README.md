# Monitor registry та reconciliation: R6

Перевірено робоче дерево після 0bb769c з Monitor CRUD та керованим reload.
critical.log: один real PostgreSQL/HTTPS тест. Нові cases: create/duplicate,
metadata/spec generation, resourceVersion/ETag і stale update, label selector,
validation/dry-run без mutation, history pagination, захист active Monitor,
standby delete/recreate з новим uid, PAT resources:write без system:admin,
відмова read для write-only PAT. Live Assessment SQL gate відхиляє свіжий
старий healthy після зміни desired та не перейменовує старий snapshot у нову
ревізію. Попередні users/identity/PAT cases також проходять.

live.json: production image (image.txt), приватний project nmo-resources-r6,
prefix 172.24. Реальний REST PUT змінює expectedStatus HTTP-перевірки 200→503.
Controller перечитує PostgreSQL, mTLS Agents отримують новий план і виконують
probes, Assessment нової ревізії стає down. Після process restart збережений
desired лишається чинним. Другий PUT повертає початковий manifest, agents
відновлюють healthy; SQL перевіряє, що evidence поточної оцінки не містить
спостережень інших ревізій. Browser sessions/PAT/CLI переживають reload та
restart; попередні users/password/history сценарії теж проходять.

```sh
python3 scripts/identity_smoke.py
python3 scripts/lab.py prepare --project nmo-resources-r6 --prefix 172.24
python3 scripts/lab.py up --project nmo-resources-r6 --prefix 172.24
python3 scripts/lab_sessions.py --project nmo-resources-r6 --prefix 172.24 --output .local/resources-live.json
```

Потрібний новий власний disposable project без users. Credentials створюються
лише в пам'яті/захищеному stdin і не потрапляють у докази. React TypeScript/Vite
та Go build/vet проходять; browser DOM clicks не автоматизовані. Це один active
Monitor/fixed roster, а не всі kind або batch apply з повного v1alpha1.
