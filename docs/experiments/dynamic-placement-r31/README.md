# Dynamic regular placement (R31)

Дата: 2026-11-16 UTC. Source: 1f98f6f. Real Docker Controller/PostgreSQL,
two mTLS Agents, controlled HTTP target; trusted HTTPS API and normal PAT CLI.
This scenario did not simulate Agent telemetry or probe results.

live.json records the successful final workflow. Two Monitor with one shared
roundRobin policy received branch and dc respectively, both produced healthy
real HTTP observations, and config.apply completed after the derived placement
revision and Agent ACK. Explicit Controller restart preserved assignments and
policy cursor state. Stopping branch caused dynamic-a to move to dc while
dynamic-b retained its effective revision. Source resourceVersions did not
change. The two-Monitor baseline recovered; loss of monitoring was not treated
as a confirmed target down. Returning branch did not move feasible assignments
back automatically.

A policy requesting two replicas from the sole permitted dc Agent stayed
pending, created zero observations and did not complete apply. A separate
Monitor with omitted strategy and one permitted Agent used default leastLoaded
and produced a real healthy probe. Unit tests additionally exercise nonzero
active/capacity ratios and tie order; this short live scenario is not a
performance benchmark or a load distribution study.

All created Monitor and the shared policy were removed using authenticated
resource operations. The original research-web Monitor returned to healthy.
image.json records the exact running image, absence of host publishing, zero
remaining regular placement selections and baseline state after cleanup.
The owned test stand was stopped with named volumes preserved. No tokens,
cookies, passwords, private PKI or DSNs are included in these artifacts.

critical-go.log records passing selection, compiler target-change invalidation,
Agent/transport, resource/config and storage package checks. postgres.log records
six real PostgreSQL test entry points passing, including an isolated-schema
case for shared roundRobin cursor, persistence, failover, per-Monitor revisions
and pending without partial selection. The existing auth test also invokes
resource/operation/diagnostic integration helpers. Go vet, eleven Python checks,
Docker production build and git diff --check passed. Unconfigured host Go tests
skip the PostgreSQL cases; the separate real database run is the evidence for
those checks.

The final image includes a guard binding cached selections to the full expanded
configuration template, not only policy hash. A changed target/runner/profile
withdraws the old selection before a new security/capability-filtered choice.
A critical compiler test verifies this behavior. Candidate security filtering
uses the existing runner IP-literal policy; Agent independently enforces it.

Earlier harness runs exposed an incorrect operationId lookup (the actual DTO
field is id) and a safe GET colliding with runtime reload during cleanup. These
were corrected without replaying DELETE. Only the final successful run is
reported here. No browser interaction was performed in this iteration; React
uses the existing placement preview DTO and Agent capability view.

Reproduce on a new owned stand:
python3 scripts/lab_sessions.py --project YOUR_PROJECT --prefix YOUR_PREFIX
--dynamic-only --output UNUSED_RESULT_PATH.json
See docs/deployment/dynamic-placement.md and ADR 0012 for behavior and limits.
Fixed roster, one Agent per zone, sticky selection, global runtime reload and
15-second startup grace remain deliberate constraints. Same-zone voting,
selectors/failureDomain/per-check overrides, full default placement without an
explicit policy, enrollment and storm/load acceptance remain open.
