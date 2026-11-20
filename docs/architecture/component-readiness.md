# Component readiness: Agent → Controller → admission → API/React

External Component bindings now have a session-scoped readiness protocol.
`components:external-v1` identifies external execution support;
`components:readiness-v1` identifies the new heartbeat telemetry. Controller
requires both for Plans containing explicit Component bindings. Old Agents
must be upgraded with Controller; legacy Plans without bindings keep working.
There are no new user YAML fields and no change to default Plan hashes.

Agent polls the declared endpoint `GET /-/healthy` every five seconds, in a
worker separate from the stream, lease renewal and regular/diagnostic probes.
It uses the same private-IP dialer as external execution: no public/metadata IP,
proxy or redirect. One request has a one-second timeout, response headers/body
are bounded to 4096 bytes and the collection round to two seconds. Shared
endpoints across multiple Monitor assignments are checked once per round;
concurrency is bounded to four. Disabled bindings make no health request.
Response bodies are discarded. Health traffic does not invoke `/probe`, send a
target parameter or consume additional-check/estimated target-byte budget.

Each heartbeat sample contains the applied Plan revision, Component ref,
binding hash, state/reason and physical check time. Only assignments applied
in the current stream are reported. Samples expire after 15 seconds; at most
five seconds of future clock skew is accepted. Suspended/expired assignments
remain subject to the existing wall/boot-time lease checks.

| Reported state | Reason | Meaning |
|---|---|---|
| ready | endpoint_ready | Local endpoint returned a bounded HTTP 200 |
| unavailable | endpoint_unavailable | Health request failed, was denied or returned another response |
| disabled | binding_disabled | Controller binding explicitly disables the exporter |
| unknown (API projection) | not_reported / stale_report | No usable current session report |

Controller validates at most 100 reports, duplicate revisions, current stream
assignment, exact ref/hash, enabled/state consistency, closed reason codes and
physical timestamp. Invalid reports fail the stream. A heartbeat for an already
withdrawn but previously applied Plan is ignored, without carrying readiness
into the replacement Plan. The report map is replaced on each heartbeat: a
missing sample never retains an old positive claim. Reconnect/new generation
starts with no readiness. Readiness is not persisted or reconstructed from old
Observations and never changes configuration/resourceVersion/effective hash.

A fresh positive report is necessary for diagnostic Blackbox candidates using
explicit bindings. Auto-trigger waits for an eligible placement/capability path
before opening an incident run. Planner eligibility consults the current
Controller session, not only its original executor catalog. Shared admission
and the bound runner recheck immediately before physical task reservation.
If no admissible candidate remains, manual start returns an inconclusive run
with zero additional checks/tasks. This makes the operator's attempted run
reviewable without charging a physical probe. There is no refund of a physical
reservation made while readiness was valid, even if the exporter then fails.

Regular checks continue during an outage and record technical unknown; they
reveal both outage and recovery. Native HTTP and legacy local execution do not
require external readiness. Disabled configuration blocks external execution
regardless of telemetry; a health report grants no permission outside the
Controller-owned Plan, Agent capability or target CIDRs.

GET /api/v1/agents now projects per-Monitor Component state, revision, reason
and checkedAt separately from Agent online state and declared capabilities.
A disconnected/expired/stale session cannot display an enabled binding as
ready. Disabled is a known configuration state even without telemetry. React
Agents shows the same data; CLI get agents includes it in JSON.

This is endpoint process readiness, not proof that every generated module is
loaded, that ICMP permissions exist or that the monitored target is healthy.
Missing modules and failures after the health check remain technical unknown
and consume an already reserved physical attempt. Independent module inventory
attestation and exporter installation/lifecycle are not part of this protocol.

[Live evidence](../experiments/component-readiness-2026-11-20/README.md) includes
real endpoint readiness/outage/recovery, an unavailable-endpoint manual run
with zero PostgreSQL physical reservations and updated API projection.
