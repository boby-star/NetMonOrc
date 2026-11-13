# Межа lease під час призупинення ОС

Linux Agent перевіряє дві часові межі перед новими регулярними й
діагностичними запусками: process monotonic deadline та CLOCK_BOOTTIME.
Другий годинник враховує suspend, тому пауза CLOCK_MONOTONIC не продовжує
дозвіл на нові probes. Обидві межі встановлюються при отриманні нового
assignment з тим самим expiresAt і запасом clock uncertainty 5s.
Неможливість отримати CLOCK_BOOTTIME закриває дозвіл на запуск.

Після startup exporter lease перевіряється повторно; startup має власний timeout 5s; регулярний exporter живе в контексті
процесу Agent, щоб завершення одного probe не зупиняло спільний exporter. На інших ОС цей Agent-профіль
відхиляє assignment: portable fallback, що пропускає suspend, не застосовано.
Saved assignment ніколи не відновлюється після рестарту процесу/ОС.

Це перевірка дозволу перед новими запусками, не firewall. Вона не обіцяє
переривання вже відкритого socket в ту саму мить, коли ОС прокидається.
Фактичний suspend хоста в Cloud не виконувався. Unit-перевірка моделює
розходження process monotonic і BOOTTIME; real-process smoke перевіряє
нормальний прийом assignment, reconnect, replay та TTL з новим guard.
