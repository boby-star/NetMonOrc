# Проверка серверной установки — 2026-11-24

Новые `deployment/server.compose.yaml`, `deployment/agent.compose.yaml` и
`examples/server-http.yaml` проверены с существующим runtime image приложения
из commit `8e2356f`. Исходники приложения не менялись; повторная сборка Go/React
для этой документационной итерации не выполнялась. Версии инструментов, image ID
и SHA-256 шаблонов находятся в `runtime.json`, результат — `verification.json`.

Проверены:

- синтаксис shell-блоков README, локальные ссылки и `docker compose config`;
- точные команды OpenSSL из README: serverAuth, clientAuth и цепочки CA;
- offline CLI validation серверного Monitor;
- PostgreSQL 17.6 с password-file, внутренней сетью и постоянным volume;
- Controller с bind-mounted config/data, непривилегированным UID 10001 и HTTPS;
- bootstrap admin, повторный bootstrap без изменения пароля, вход/выход;
- HTTP 401 без сессии, HTTP 200 и React index/assets после старта;
- два отдельных Agent-процесса с собственными cert/key/spool и mTLS;
- реальные native HTTP probes: unknown до Agents → healthy, обе зоны покрыты;
- restart Agent и Controller, переподключение и сохранение пользовательской сессии.

Тест выполнен на одном Linux Docker host. Agents подключались к отдельной
тестовой сети цели и control bridge Controller, а не через внешнюю сеть между
физическими серверами. HTTPS проверялся через опубликованный loopback port с
проверкой CA/hostname. Тестовые config-каталоги имели 0750/группу оператора,
поскольку клиент Docker работал без sudo; data-каталоги — 0700, UID/GID 10001.
В README для rootful установки с sudo используются приватные config 0700.

Установка apt-пакетов на чистой Ubuntu, systemd, внешний firewall/маршрутизация,
другие архитектуры, rootless Docker, ICMP capabilities и browser interactions
в этой проверке не выполнялись. Наличие Blackbox 0.27.0 проверено через `--version`;
новые TCP/DNS/ICMP trials не проводились.

Созданные тестовые контейнеры и сети остановлены/удалены. PostgreSQL volume,
каталоги config/data и PKI сохранены в `.local/server-deploy-check-20261124`.
Секреты в пакет доказов не включены. Это проверка указанного механизма запуска,
а не подтверждение production HA или полного старого ТЗ.
