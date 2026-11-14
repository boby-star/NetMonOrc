# Manual diagnostic start/cancel R13

Робоче дерево після f5aaf22, дата 2026-11-14. Production image та fixture image
записані в image.txt. Ізольований Compose project nmo-diagnostic-control-final-r13,
private prefix 172.19. PostgreSQL, два справжні mTLS Agent, без host ports.

scripts/lab_sessions.py створює окремий fixed profile manual-slow з native HTTP
probe на приватний /slow (відповідь через 12s), запускає його через CLI,
чекає accepted фізичної задачі та надсилає public cancel. Observation отримано
в межах 8s після cancel, раніше звичайної відповіді. Task/session лишаються
cancelled, used budget=1. Чинний regular Monitor залишається healthy.
Після process restart той самий requestId повертає ту саму cancelled session;
кількість runs не збільшується. Disabled binding відхиляє новий manual start.

Після restart helper чекає свіжий Assessment і heartbeat/applied revision
обох Agent, що підтверджують готовність нових streams.

Критичний real PG/HTTPS use case перевіряє transactional admission/recovery,
конфлікт reused key/body, cooldown і episode після cancel, stale revision,
незмінний budget, late receipt/Observation і заборону overwrite terminal state.
Agent receipt/lease і disk spool/backpressure use cases пройшли з дозволеними
localhost sockets. Go build/vet, тести зачеплених пакетів і React build пройшли.
Browser DOM clicks не автоматизовані; формули planner не змінювалися.

Для повторення потрібен новий власний disposable project без users,
lab.py prepare/up і lab_sessions.py з його project/prefix. Named volumes
зберігаються після down. Cookie/PAT/password/DSN/private keys не експортуються.
Межі deployment: один active Monitor/fixed roster; full placement/enrollment
та решта DSL catalog залишаються відкритими.
