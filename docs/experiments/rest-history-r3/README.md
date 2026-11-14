# Інтеграція пароля, історії та React (R3)

Перевірено робоче дерево після 4120efa з історією спостережень.
critical.log — один критичний HTTPS/PostgreSQL тест: identity lifecycle,
зміна пароля, відкликання сесій, TTL/lockout, keyset з однаковим часом,
фільтри та некоректні запити. Команда: `python3 scripts/identity_smoke.py`.

live.json — реальний приватний Compose project nmo-auth-r3 (172.27),
Controller + PostgreSQL + два mTLS Agent + Blackbox + production React assets.
Сценарій: bootstrap, HTTPS login, live Assessment, історія/пагінація/зона,
зміна пароля, відмова старому паролю та cookie, новий вхід, restart Controller,
відновлення сесії, CSRF та logout. Команди:

```sh
python3 scripts/lab.py prepare --project nmo-auth-r3 --prefix 172.27
python3 scripts/lab.py up --project nmo-auth-r3 --prefix 172.27
python3 scripts/lab_sessions.py --project nmo-auth-r3 --prefix 172.27 --output .local/session-history-live.json
```

Потрібний новий власний disposable project без users. Паролі генеруються
в пам'яті; ці облікові дані не призначені для людського встановлення.
Наявні користувачі не перезаписуються. Credentials у доказах відсутні.
SHA256 образу — image.txt. React production build успішний; автоматичних
кліків у браузері не виконувалось. Це перевірені сценарії, не 100% всієї специфікації.
