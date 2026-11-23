# Установщик master/agent — 2026-11-24

`install.sh` — один самостоятельный Bash-файл со встроенным Python (стандартная
библиотека) и Compose-шаблонами. Проверка использовала существующий runtime image
приложения; Go/React исходники в этой итерации не менялись. Версии и source hashes
находятся в runtime.json, результат выполнения — result.json.

На настоящем PostgreSQL, Controller и двух Agents пройдены 15 сценариев:

- master install и bootstrap администратора через защищённый stdin;
- повторный master install сохраняет credentials и существующий контейнер;
- два Agents с mTLS и настоящими HTTP probes достигают healthy в обеих зонах;
- повторный запуск сохранённой standalone-копии скрипта сохраняет Agent;
- offline --no-start готовит файлы и не запускает сервисы;
- некорректный override/CIDR и чужая Agent identity отвергаются без изменения state;
- смена cert/key без --rotate-tls отвергается;
- явная ротация leaf cert с той же identity пересоздаёт контейнер и сохраняет spool identity;
- чужой unmanaged каталог не присваивается установщиком;
- Compose override сохраняется после update;
- смена image tag сохраняет пароль БД, данные и пользовательскую сессию;
- stop/restart сохраняет spool identity;
- допустимый 63-символьный Agent ID получает стабильное имя project длиной 63.

Смена image в тесте — alias того же runtime image, а не проверка совместимости
разных версий приложения или схем БД. Для проверок использовался root driver
container с host network, локальным Docker socket и host Bash/OpenSSL/Docker CLI.
Работа установщика была ограничена собственными projects и каталогами
`.local/installer-check*-20261124`. gRPC проверялся через опубликованный private
bridge-IP host port, HTTPS — через loopback с CA/hostname verification.
Это один Docker-host, а не отдельные физические серверы.

Проверены Bash/Python syntax, shell-блоки README и точное совпадение встроенных
Compose-шаблонов с deployment/server.compose.yaml и deployment/agent.compose.yaml.
Action status проверен на сохранённых остановленных сервисах. После проверки
тестовые services остановлены через action stop. Containers,
networks, PostgreSQL volumes, config/history, PKI и spool сохранены. Удалён только
собственный HTTP fixture без постоянных данных. Secrets/stdin/cookies в отчёт
не включены; публичные JSON содержат только результаты и read models.

Установка apt-пакетов на чистой Ubuntu, интерактивный TTY prompt, build/pull/image
archive branches, browser interactions, rootless/remote Docker, внешняя сеть,
HA и автоматическая выдача cert не проверялись. Rootless/remote contexts
установщиком отвергаются. Финальные правки проверки Docker context, обработки
ошибок и bootstrap зависимостей проверены синтаксически; apt на host не запускался.
