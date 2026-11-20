# Declarative external Component

`Component` is the eleventh registered v1alpha1 kind. REST CRUD/history,
config validate/plan/apply, offline YAML validation, CLI get and React resource
editing use the same closed compiler. This implements the external exporter
binding in DSL §17; it does not install containers or accept arbitrary plugins.

```yaml
apiVersion: netmonorc.io/v1alpha1
kind: Component
metadata: {name: local-blackbox}
spec:
  driver: blackbox
  mode: external
  agentSelector:
    matchLabels: {role: probe}
  endpoint: http://blackbox:9115
  moduleSet: standard-network
  enabled: true
```

`driver` is blackbox only, mode defaults to external, moduleSet to
standard-network, enabled to true. agentSelector is required; {} matches the
trusted roster. Labels come from Controller -agent-labels-file, never Agent
Hello. MatchExpressions supports In/NotIn/Exists/DoesNotExist with canonical
ordering. Overlapping selectors, including disabled bindings, are rejected
against the known roster. Offline validation needs --slots and
--agent-labels-file to validate actual selection/overlap.

Unknown fields, null fields, malformed types, managed mode, Docker image/shell
fields and unknown module sets are rejected. Description is bounded and does
not change execution identity. Endpoint must be http without credentials, path,
query or fragment: a single Docker service name, private IP or loopback IP.
Public IP/FQDN and metadata/link-local addresses are forbidden. The Agent checks
every resolved endpoint IP and dials the selected private literal, without a
second DNS lookup, proxy or redirect. Probe target CIDRs remain a separate gate.

## Preparing standard-network modules

The pinned release catalog is the existing closed, validated Blackbox module
translator. It supports the same fields as the local prepared runner. External
modules are content-addressed: nmo_SHA256 of standard-network/v1 plus canonical
module JSON. IDs include timeout/protocol/options; target addresses enter the
module only where they affect protocol configuration. Identical regular and
diagnostic modules share an ID; changing probe semantics creates another ID.

Prepare the existing exporter before applying its binding:

```sh
netmonorc component-config -f catalog.yaml > external-blackbox.json
blackbox_exporter --config.file=external-blackbox.json --web.listen-address=:9115 --timeout-offset=0
```

Bind exporter only inside the Agent's private Docker network; do not publish it
as a public probe service. catalog.yaml contains the Monitor/Target/CheckProfile
and any diagnostic profiles that may be selected, including manual alternatives.
The CLI resolves the supplied catalog using the shared offline compiler and
prints bounded exporter JSON. It performs no network call, container install or
exporter reload. It includes every supplied diagnostic profile for each supplied
Monitor target context. Missing references remain errors. No Monitor or no
Blackbox checks is an error. Native HTTP needs no external module.

After changing check parameters, prepare/reload the exporter configuration
through deployment, then apply the matching manifests. The Component apply
operation changes binding only. A missing module, incompatible exporter or
unreachable endpoint gives a technical unknown, never a replacement default
module or a negative target probe. Native extraction still uses native HTTP.

## Runtime, compatibility and reconciliation

The Controller snapshots matched Agent/ref/hash/endpoint/moduleSet/enabled into
Plan. Binding changes alter the effective Monitor revision and invalidate
cached placement templates without rewriting Monitor source version/history.
New Agents advertise components:external-v1 and components:readiness-v1; older Agents cannot accept Plans
with binding snapshots. Upgrade Agents with Controller before applying them.

For an explicitly matched binding, regular and diagnostic execution use the
external endpoint. Disabled bindings exclude Blackbox diagnostic candidates and
produce regular execution_failed/unknown/runner_unavailable. Missing exporter
or module produces technical unknown. Regular probes continue to reveal outage
and recovery; advertised driver support is not a continuous health report.
A separate [proactive readiness protocol](../architecture/component-readiness.md)
now polls the endpoint and gates diagnostics on a fresh, session-bound report.
This is process readiness; it does not attest every loaded module.

Compatibility is explicit: Agents without a matching Component retain the
previous prepared local binary. Adding a Component overrides only matched
Agents. Disabling that Component does not restore the local binary. Deleting
it or changing its selector so the Agent no longer matches restores legacy
local execution. This migration behavior differs from a strict deployment in
which all Blackbox capability requires an explicit Component manifest.

The Agent does not manage external exporter lifecycle. Closing a binding cannot
stop/delete its container. Existing local-mode Plans serialize identically.
Current authenticated Observations retain their revision and historical source
ownership through updates/restart.

[Acceptance evidence](../experiments/components-2026-11-20/README.md) covers real
regular/diagnostic execution, outage without local fallback, disabled binding,
restart and source-preserving deletion. Production React build passed; no new
browser interaction is claimed.
