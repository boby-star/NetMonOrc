# Live diagnostic estimated-byte budget

`DiagnosticProfile.spec.budget.maxEstimatedBytes` now works in the Controller,
in addition to the pure planner. It is optional and positive. Unknown check
estimates are rejected during live profile compilation. Check/count/duration,
Agent capacity and target policy still apply independently.

```yaml
budget:
  maxAdditionalChecks: 3
  maxDuration: 60s
  maxParallel: 1
  maxEstimatedBytes: 50000
```

This is the deterministic `protocol-envelope-v1` planning model required by DSL
§11.2 budget semantics, not a packet capture, actual billing or a hard wire limit.
Configured response bounds are included before execution. Numbers are explicit
policy assumptions, not experimental measurements:

| Check | Estimated bytes per physical attempt |
|---|---|
| HTTP | 32768 + maxBodyBytes + URL bytes + header name/value bytes (+4 per header) |
| HTTP with redirects | 10 times the HTTP estimate (at most ten requests) |
| DNS | 8192 |
| TCP | 4096 |
| ICMP | 2 × (payloadSize + 128) |

HTTP envelope includes nominal headers/TLS setup; protocol retransmits, actual
certificate chains, resolver/exporter/control-plane traffic and network framing
may differ. No estimate is described as a proven upper bound on wire traffic.

The coordinator uses the same estimator as PostgreSQL admission. Selection
excludes estimates greater than the remaining budget. Each retry spends its own
estimate. Before a task is sent, ReserveDiagnostic locks the run row, recomputes
the estimate from the compiled check, verifies the remaining budget and writes
the task + count + bytes in one transaction. Concurrent callers cannot spend the
same remainder. A rejection, cancellation, lost frame or restart never refunds
a reservation. The existing one-run-per-incident fence prevents restart from
silently obtaining a fresh budget.

New additive columns keep the declared limit, consumed estimate and task
estimate. Existing runs without a byte limit remain unchanged and expose no
invented estimate. REST session budgetUsed.estimatedBytes and task.estimatedBytes
are optional; React labels them as estimated. JSON/CLI evidence export preserves
the public session/task fields. Terminal diagnostic reports distinguish planner
estimatedBytes from durable reservedEstimatedBytes, which can differ after a
transport failure. Cost units remain separate.

[Real acceptance](../experiments/byte-budget-2026-11-19/README.md) verifies a
budget below one attempt (zero dispatch), an exact one-attempt limit (one real
Observation, second Fixed step blocked) and persisted accounting after restart.
A real PostgreSQL concurrency test verifies exactly one of two simultaneous
reservations, public task/session accounting and no refund after rejection.
