# Dependency graph and panel stability (R28)

Дата: 2026-11-16 (Asia/Jerusalem). Source d2cbc91, Chromium 154.0.8037.57,
Node 24.19.0. Browser scenario uses production React assets and a loopback
read-only API fixture. No real passwords, PAT or cookies were used. This is
not live Controller/TLS/PostgreSQL/Agent authentication integration.

Chromium exercised directed SVG arrows, keyboard node selection, incoming
and outgoing links, relation/strength details, literal HTML-like description,
current-page search, opaque cursor pagination, 503 clearing old diagram,
manual recovery, empty catalog and scope-based UI visibility. Mutation requests
and runtime console exceptions: zero. Scope visibility is a client UI check,
not a claim of server RBAC enforcement. Fixture includes permitted relatedTo
cycles; UI does not reinterpret them as a requires DAG.

Browser testing exposed duplicate sibling keys in App: clock ticks could
multiply Configuration/Dependency sections and reset interaction state.
Component-prefixed stable keys fix the problem. The successful scenario waits
through multiple clock updates before and after keyboard selection, checks one
configuration/graph panel and retained selected node. Initial failing fixture
runs identified this real frontend defect; they are not counted as passed.

panel.png is the actual rendered fixture panel, with no private customer data.
The production Docker build passed. A stopped isolated container was created
from that image only to read packaged web assets; no service was started.
SHA256 of every packaged HTML/JS/CSS file exactly matched the browser-tested
host build. assets.json records that comparison, image.txt records the image.
The temporary image-check container was removed without touching existing
stand volumes. Go/API algorithms and Agent transport did not change in R28.

Reproduce: npm run build --prefix web, then
node scripts/dependency_browser_smoke.mjs NEW_RESULT_JSON NEW_SCREENSHOT_PNG.
Use new output paths; requires installed Chromium and Node 24+. Chromium uses
an isolated temporary profile and no sandbox for this local fixture only;
profile is removed on completion. No third-party test dependency was added.

Limits: at most the loaded API page (20 links), not full catalog graph; manual
refresh, no state/causality inference. This does not repeat earlier live tests;
see dependencies-r27 for separate real PG/mTLS integration evidence. Full
multi-Monitor, dynamic placement/replicas and Agent lifecycle remain open.
