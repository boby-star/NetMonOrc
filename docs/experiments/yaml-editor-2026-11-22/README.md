# Одиночний YAML-редактор — натурне приймання

Owned project nmo-yaml-editor-20261122, HTTPS лише 127.0.0.1:8449, справжні
PostgreSQL/mTLS Agents/fixture. Розширено попередній браузерний runner режимом
--single-editor. Спочатку через пакетний UI створюються Target/CheckProfile/
Monitor; YAML Monitor виконує реальний native HTTP probe з branch.

live.json підтверджує попередній повний package workflow та нові кроки:

- Target відкрито через select/name/«Завантажити» в одиночному редакторі;
  textarea містить точні 1.0 і 9007199254740993;
- замість JSON вставлено YAML, натиснуто validate/plan/«Зберегти поточний»;
  після відповіді textarea знову має точні числа з server source;
- незмінний Target spec не змінив effective revision залежного Monitor;
- YAML-чернетку перенесено кнопкою в пакет; source numbers і expectedVersion
  завантаженого Target збережено;
- сторонній REST apply змінив Target criticality з 3 на 4;
- «Зберегти поточний» зі старою версією дав 412, показав конфлікт і не змінив
  історії; «Завантажити» показало актуальний source з criticality=4;
- PostgreSQL має рівно 3 Target source revisions: create, single save,
  сторонній apply. Відхилена заміна не створила четверту. Monitor source
  history лишилась 1; той самий idempotent package replay має 1 operation.

browser-single.png переглянуто: точні числові літерали видимі в textarea,
разом з актуальною resourceVersion. browser.png показує успішний пакетний apply.
Сценарій також зберігає перевірки integer 200.0 rejection, duplicate key,
CSRF 403, anonymous 401 та healthy від реального probe. Runtime exceptions=0.

Після браузерного trial formatter додатково прибирає непотрібне HTML escaping
в текстовому document, щоб великий рядок '<' не роздував source за request
limit. Це підтверджено unit test. Фінальний image повторно запущено: новий
healthy Assessment, точний HTTPS source та відповідний ETag підтверджені у
final-smoke.json. Повний browser flow на цьому image повторно не запускався.
runtime.json розділяє trial/final image.

Go/vet та production React build пройшли. Звичайні DSN-залежні DB tests у
go.log пропускаються; PostgreSQL твердження вище перевірені натурним helper.
Unit tests перевіряють незмінність hash/чисел/stable formatting, MIME та
відхилення batch conditions одиночним CRUD, спільність source/ETag. Logs
нормалізовано лише trailing spaces; SHA256SUMS фіксує артефакти.

Браузер — headless Chromium зі SPKI pin лише owned Controller; Node окремо
перевіряє TLS CA/hostname. Глобальне сховище довіри не змінюється. Пароль
demo admin і PKI лишаються у приватній .local й не входять до артефактів.
Початкова спроба prefix=172.15 відхилена helper як non-RFC1918 до створення
стенду; підтверджений trial використовує 172.24. Стенд зупинено, .local/PKI/
volumes збережено. Це один Docker host, не HA/scale або всі види DSL fault trial.

```sh
python scripts/lab_yaml_web.py prepare --project YOUR_NEW_PROJECT --prefix YOUR_PRIVATE_PREFIX --port YOUR_FREE_PORT
python scripts/lab_yaml_web.py verify --project YOUR_NEW_PROJECT --prefix YOUR_PRIVATE_PREFIX --single-editor --output .local/YOUR_NEW_OUTPUT/live.json --screenshot .local/YOUR_NEW_OUTPUT/browser.png
python scripts/lab_yaml_web.py down --project YOUR_NEW_PROJECT --prefix YOUR_PRIVATE_PREFIX
```

[Контракт](../../deployment/yaml-editor.md).
