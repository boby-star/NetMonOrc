# Diagnostics toggle R9

Робоче дерево після 97a53ca. Приватний Compose project nmo-toggle-r9,
prefix 172.21, production image (image.txt). CLI apply --wait перемикає
binding enabled false/true. Regular probes і Assessment тривають. HTTP fault
при disabled дає down без приросту nmo_diagnostic_runs; після re-enable
аналогічний fault запускає нову автоматичну diagnostic session. Сервіс
відновлено до healthy. Попередні CLI/apply/ACK/replay/users/PAT/revision cases
також проходять. Credentials не включені в докази. Browser DOM clicks не
автоматизовані. Наявність нового run не означає перевірений RCA outcome.

Відтворення: новий власний disposable project без users, scripts/lab.py
prepare/up та scripts/lab_sessions.py з project/prefix. Дані існуючих
інсталяцій не перезаписуються.
