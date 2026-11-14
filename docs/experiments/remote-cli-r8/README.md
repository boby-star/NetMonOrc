# Remote CLI R8

Робоче дерево після 2dfc0f4. security.log — існуючий критичний тест довіреного
TLS, відмови redirect/plaintext/public credential file/symlink після переходу
на спільний client transport. live.json — приватний project nmo-cli-r8,
prefix 172.22, image.txt. Справжній netmonorc validate --remote читає YAML
із Controller volume; plan і apply --wait передають ConfigBatch через stdin.
Apply чекає фізичних ACK та завершується completed. Після process restart
CLI знову автентифікується тим самим PAT і get читає persisted completed;
зворотний apply --wait відновлює початковий стан. Попередні критичні users,
PAT, history, revision та idempotency сценарії також проходять.

Для відтворення потрібні новий власний disposable project без users,
scripts/lab.py prepare/up та scripts/lab_sessions.py з project/prefix.
Credentials не включені в докази. Browser DOM clicks не автоматизовані.
