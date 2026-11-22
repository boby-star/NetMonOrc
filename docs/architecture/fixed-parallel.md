# Parallel Fixed DAG with deterministic waves

`DiagnosticProfile` with `mode: fixed` and `budget.maxParallel > 1` now runs
independent steps concurrently, as required by DSL §11.5. No new YAML enum or
shell/action driver is introduced. Default maxParallel=1 retains serial
execution. Adaptive and Static remain serial within the declared upper bound.
[Lab example](../../examples/diagnostics/parallel-fixed.yaml).

```yaml
mode: fixed
budget:
  maxAdditionalChecks: 3
  maxDuration: 60s
  maxParallel: 2
fixed:
  steps:
    - {id: left, actionRef: from-office}
    - {id: right, actionRef: from-server}
    - id: join
      actionRef: verify
      dependsOn: [left, right]
      when: 'tasks["left"].outcome == "success" && tasks["right"].outcome == "success"'
```

The example assumes three declared actions. Agent placement remains explicit
and subject to capability, target policy, readiness and global shared admission.
At most one diagnostic activation per Agent enters a wave; its regular slot
remains available. maxParallel is a maximum, not a promise to use unavailable
capacity. Steps on the same Agent are serialized even when their graph nodes
are independent.

## Scheduling and publication

At each barrier the planner rebuilds hypotheses from currently usable physical
evidence, sorts runnable Step IDs and evaluates guards against a snapshot of
terminal predecessor outcomes. A false guard skips the branch. Unknown/error
blocks it and marks incomplete evidence. Children do not see a partially
finished wave. An exhausted physical budget still permits zero-cost guard
skips, so a legitimate skipped tail does not falsely block completion.

The scheduler allocates remaining count/estimated-byte quotas in canonical
Step ID order, then obtains shared admission permits in global Agent ID order.
This prevents hold-and-wait cycles between concurrent multi-Agent sessions.
Waiting costs no task/check/cost/bytes and remains inside the same deadline.
If evidence expires while waiting, permits are released and the wave is
replanned before dispatch. Readiness/eligibility and remaining safe time are
rechecked at execution. No manifest controls executable paths or side effects.

Runner/EvidenceRunner implementations must allow concurrent Execute calls when
Fixed opts into maxParallel>1; the live transport runner and read-only replay
runner satisfy that contract.

Workers execute only physical attempts. They never mutate shared ledger,
hypotheses, tasks or journal. The planner waits for all workers, then accounts
for preallocated quotas and publishes decisions in canonical wave/Step order.
Each wave's hypothesesBefore/gain uses the same pre-wave evidence snapshot.
Downstream decisions use the complete rebuilt context. Physical timestamps,
TTL/maxSkew and independent-domain confirmation remain authoritative; stable
journal ordering does not fabricate equal durations or independent evidence.

A technical retry keeps the same action/Agent/Step and increments attemptNo.
Retries join subsequent barrier waves and spend count/cost/time/estimated bytes
again. No completed success/failure/unknown is retried automatically. Competing
retries cannot race for the last budget token: the same Step ordering allocates
it. Fixed cannot become completed while required runnable steps remain blocked,
a guard lacks context or terminal evidence is technically unavailable.

Cancellation propagates to every worker, waits for worker exit and prevents
new child waves. The live Agent cancellation path stops the physical probe;
already reserved attempts remain spent. A lost reply can leave physical
completion unknown; existing detached admission/deadline fences remain intact.

## Durable admission and provenance

PostgreSQL stores max_parallel in each run snapshot (legacy default 1).
ReserveDiagnostic locks the run row, checks current unexpired reserved/accepted
tasks, count, estimated bytes, deadline, revision and Agent generation, and
inserts/increments in one transaction before sending. Concurrent callers cannot
exceed maxParallel. Rejected/finished tasks release active parallel capacity
without refunding consumed physical count/bytes. Terminal/cancelled run rows
still fence further reservations. Old Controller restart behavior remains
inconclusive without replenishing the incident budget.

New fixed tasks and decisions preserve stepId. This distinguishes two nodes
using the same actionRef and links each physical task/Observation to its actual
DAG activation. REST detail/tasks, CLI JSON export and React show the field.
stepId is dispatch metadata, excluded from the compiled profile hash. Legacy
rows without it remain unspecified; no historical Step is guessed from Action.
Default Plan/profile serialization and source resourceVersions are preserved.
Migrations add columns only; they do not remove data or volumes.

[Acceptance evidence](../experiments/fixed-parallel-2026-11-20/README.md) includes
real overlapping probes, reversed physical completion order with stable logical
journal, downstream dependency, count/byte quotas, two-worker cancellation and
actual PostgreSQL concurrent reservation tests. No new browser interaction or
multi-host fault-domain independence is claimed.

## Scoped condition context

Fixed conditions now receive only completed direct/transitive predecessors,
for both serial and parallel execution. Static references outside dependsOn
are rejected; computed keys see the same restricted context. Task/monitor
fields are typed. See [validation and compatibility](../deployment/fixed-context.md).
