# Same-zone physical replicas (R32)

Дата: 16.11.2026 (Europe/Kiev). Source: 81316fe. Real private Compose stand
with Controller, PostgreSQL and two separately authenticated mTLS Agents.
Controller assigns both Agent identities to administrative zone office; their
network namespaces remain separate. This is not proof of independent failure
domains or separate physical hosts.

live.json records seven successful phases. Both physical sources initially
returned healthy HTTP results. Prohibiting only the branch target route made
one probe negative while dc remained successful. With planned N=2:

| Phase | State | Physical coverage | Q / H |
|---|---|---|---|
| both healthy | healthy | complete | 1 / 1 |
| mixed, all | down | complete | 1 / 0 |
| mixed, any | healthy | complete | 1 / 1 |
| mixed, quorum | unknown | complete | 0 / null |
| quorum, dc stopped | unknown | degraded | 0 / null |
| all, required failure without peer | down | degraded | 1 / 0 |
| both recovered | healthy | complete | 1 / 1 |

The quorum threshold did not shrink when dc disappeared. Divergence retained
two actual evidence IDs; the independent required failure retained one actual
negative ID. No synthetic physical observations or extra votes were stored.
User/session authentication, trusted HTTPS mutation/ETag/CSRF and logout
revocation were used normally. Test passwords/cookies existed only in memory
and stdin; none are included here. The test restored the original manifest and
network route, restarted dc, and stopped the owned stand with volumes preserved.
Host ports were not published. Browser interaction was not exercised.

critical-go.log records passing monitor/config/resource/placement/transport
and Controller package checks. These include planned-N all/any/quorum cases,
N=3 majority, duplicate deliveries, incoherent negative witnesses, newest
execution failure replacing an old vote, deferred values/metrics checked-AST
validation and distinctBy=zone filtering. The live scenario used two physical
Agents; three-Agent majority was checked in unit cases, not in a live run.
The unconfigured host storage tests skip their DSN-dependent cases; this
iteration's real SQL execution is the Compose workflow, not a new full isolated
storage-suite run. Go vet, eleven Python checks and Docker build passed.

images.json distinguishes the full-workflow image and final rebuild after a
dependency classification-only edit (the pinned version did not change).
final-https.json records final-image trusted TLS 1.3, fresh healthy one-zone
assessment with two evidence IDs, anonymous 401 and production UI assets.
That update/startup check is not another full seven-phase workflow. A final
source comment clarification does not change execution behavior.

The approved Q03 semantics were synchronized in DSL §9.7 and TZ §4.5, both
Markdown and the corresponding DOCX section. No HTTP route or DTO field was
added. CEL uses aggregate outcome/typed execution metadata; values/metrics
aggregation remains deferred and is rejected before apply. Raw per-Agent data
are retained. See docs/deployment/replica-assessment.md and ADR 0013.

Reproduce with scripts/lab_replicas.py --project NEW_PROJECT --prefix PREFIX
--output UNUSED_PATH.json. Requires the built research images, Docker Compose,
Python and OpenSSL. Use a new owned project/output; existing identities and
files are preserved. The helper writes a test-only Controller roster override
and initial manifest into that fresh stand, then removes no data volumes.

During final packaging the workspace disk filled. Only unused Docker build
cache older than 24 hours was reclaimed; code, images and named volumes were
preserved. This was an environment capacity issue, not a failed monitoring case.

Open scope: enrollment/certificate lifecycle, trusted failure-domain/label
registry, per-check placement, full compiler-default placement, independent hot
reload, browser acceptance of the expanded flows and load/storm experiments.
No 100% completion, HA or production throughput claim is made.
