# Local HTTPS release workflow (R29)

Дата: 2026-11-16 (Europe/Kiev). Source: ad97ba4. Node 24.19.0,
Chromium 154.0.8037.57; production React bundle, actual Docker Controller,
PostgreSQL, two mTLS Agents and controlled research target.

Live browser acceptance passed with normal password login and real HttpOnly
cookie authentication: Target creation, Dependency creation and directed graph,
configuration revision/Agent reconciliation, derived ResourceGroup healthy from
real observations, referenced Target delete rejected with 409, cookie session
restored after page reload, deletion of all three created resources, original
revision restored, stable panels and logout revocation. Mutation requests were
not replayed. No JavaScript runtime exceptions were observed. This is a focused
acceptance scenario, not validation of every endpoint or production load.

The browser trusted only the public stand CA in its own temporary NSS profile.
Node separately verified HTTPS with this CA and TLS 1.3. No insecure TLS flags,
passwords, session cookies or private keys are included in this evidence. The
random acceptance password travelled only through memory/stdin and was not
saved. Bootstrap repeated with a different input did not replace the account.
The screenshot contains only deliberately created demonstration resource names.

Only Controller HTTPS was published, bound to 127.0.0.1:18443. Other services
had no host bindings and were excluded from the additional UI network. A
subsequent down/up without --ui-port preserved this binding, the administrator
and named volumes, and returned to healthy. The test stand was then stopped;
its named data volumes were preserved.

Initial real browser runs exposed two launch defects: internal-only networking
could not serve the published port, and SQL preflight consumed inherited stdin.
These are corrected by the explicit Controller-only UI bridge and DEVNULL for
commands without explicit input. Repeated parallel page reads also exhausted
Controller admission after reload. Read-only requests now run serially per tab,
with at most three attempts for 429/503/network errors in a common 8s budget.
Authentication/permission failures and mutations are not retried. Failed
pre-fix runs are not counted as successful checks.

fixture-browser.json records separate Chromium error/retry/pagination cases on
an API fixture. live-browser.json records the real integration described above.
assets.json verifies all packaged HTML/JS/CSS match the browser-tested host
build byte for byte. image.txt records the local image digest. Python discovery
ran 11 checks successfully; TypeScript/Vite build, Docker build and git diff
--check passed. No Go business logic changed in this iteration.

Reproduction: follow docs/deployment/quickstart.md on a fresh owned project,
then run scripts/lab_browser.py with unused output paths. Existing users and
files are deliberately preserved. Browser prerequisites: Node 24+, Chromium,
certutil. Chromium --no-sandbox is used only for local headless acceptance.

Limits: this research profile executes one active Monitor. Full multi-Monitor,
dynamic placement/replica voting, Agent enrollment/rotation and final full
v1alpha1 acceptance remain open; this result does not claim 100% completion.
