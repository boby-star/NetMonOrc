# Multiple Monitor execution (R30)

Дата: 2026-11-16 UTC. Source: 337cedf. Node 24.19.0,
Chromium 154.0.8037.57. One real Docker Controller/PostgreSQL and two mTLS
Agents; production React assets and controlled target. No fake monitoring API.

The normal UI workflow passed: password login, Target/Dependency CRUD and
reference-delete guard, graph interaction, derived group assessment, revision
change/restoration, session reload and logout. The extended scenario then
created research-other alongside research-web through the resource editor.
Both use the same check ID http and the same real target, but different expected
status policies. research-other correctly showed down while research-web
remained healthy at its original revision. Switching Monitor did not require
another login; page reload restored both the authenticated session and selected
Monitor. Removing research-other was followed by a fresh healthy primary
assessment before logout. The script waits for new server evidence after
reconciliation rather than relying on a badge left from an earlier response.

SQL confirmed observations from two distinct Agents for each Monitor:
research-web: 42 successes, zero failures; research-other: two failures,
zero successes. These counts describe this short scenario, not a load test.
browser.json contains the safe assertions and counts. graph-panel.png is a
real screenshot of the shared Dependency panel captured earlier in the same
workflow; it is not a screenshot of both Monitor assessments.

The browser used an isolated trusted local public CA profile, with no disabled
TLS verification. No passwords, tokens, cookies, private PKI or DSNs are in
these files. The test administrator is preserved in the owned test volume;
its generated password was not exported. Created demonstration resources were
removed through the UI. Loopback-only Controller publishing and exclusion of
private services from the UI network were checked.

postgres.log records all five real PostgreSQL entry-point tests passing;
the browser-session test also exercises the resource/apply/diagnostic helpers.
The new independent-revision test confirms ACK coexistence, mixed duplicate
replay, reconnect requiring new generation ACKs and old-stream fencing.
critical-go.log includes the two-plan Agent scheduler case and HTTP revision
routing/isolation case. race.log records passing Agent/transport/HTTP race
checks. Eleven Python checks, React/TypeScript build, Docker build, go vet
and git diff --check passed.

The first PostgreSQL harness omitted a required fixture; it was corrected and
rerun successfully on a new disposable database with no host port. An initial
browser fixture used an invalid two-second interval; it was changed to the
existing ten-second DSL minimum. Another run reached both Monitor assertions
but attempted logout during shared runtime reconciliation. The final successful
run explicitly waits for fresh post-reconciliation assessment. Failed runs are
not counted as passes, and mutations are not automatically replayed.

After browser acceptance, final review moved diagnostic ACK readiness reading
inside its existing mutex. images.json distinguishes the browser-tested image
and final rebuilt image. The final image was deployed over the owned stand:
fresh healthy state, preserved administrator, strict HTTPS, anonymous 401 and
production assets passed. assets.json verifies that final packaged HTML/JS/CSS
match the browser-tested host bundle byte for byte. This final update check is
not another full two-Monitor browser scenario. The owned stand was then stopped
with named volumes preserved.

Reproduce: docs/deployment/quickstart.md, then scripts/lab_browser.py with
--multi-monitor on a new owned project and unused output paths. Requires
Node 24+, Chromium and certutil; --no-sandbox is for this local acceptance
client only. See docs/deployment/multiple-monitors.md and ADR 0011.

Limits: fixed roster; one regular and one diagnostic worker per Agent; bounded
two-exporter regular cache. Catalog changes still restart shared streams.
Independent hot reload, dynamic regular placement, same-zone replica voting,
Agent enrollment/rotation and broad performance/storm acceptance remain open.
No 100% completion or production-scale throughput claim is made.
