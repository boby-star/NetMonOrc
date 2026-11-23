package diagnostics

import "github.com/boby-star/NetMonOrc/internal/monitor"

func replicaCount(p Placement) int {
	if p.Replicas > 1 {
		return p.Replicas
	}
	return 1
}
func replicated(p Profile) bool {
	for _, a := range p.Actions {
		if replicaCount(a.Placement) > 1 {
			return true
		}
	}
	return false
}

// Select the entire planned set before any physical execution. Missing actors
// never shrink N; retries remain attached to their original physical actor.
func selectReplicas(p Placement, eligible []Executor, round int) []Executor {
	pool := append([]Executor(nil), eligible...)
	selected := []Executor{}
	for len(selected) < replicaCount(p) {
		candidates := []Executor{}
		for _, agent := range pool {
			allowed := true
			for _, prior := range selected {
				if agent.ID == prior.ID || (p.DistinctBy == "zone" && agent.Zone == prior.Zone) || (p.DistinctBy == "failureDomain" && (agent.FailureDomain == "" || agent.FailureDomain == prior.FailureDomain)) {
					allowed = false
					break
				}
			}
			if allowed {
				candidates = append(candidates, agent)
			}
		}
		if len(candidates) == 0 {
			return nil
		}
		selected = append(selected, chooseExecutor(p, candidates, round))
	}
	return selected
}

// Scalar values are never merged across physical replicas. Unanimous outcomes
// provide a summary; the raw per-Agent results remain separately addressable.
func aggregateTaskEvidence(results map[string]TaskEvidence) TaskEvidence {
	out := TaskEvidence{ReplicaResults: results, Outcome: Outcome{Execution: "completed", Outcome: "unknown"}}
	first := true
	for _, data := range results {
		if first {
			out.Outcome = data.Outcome
			out.ReasonCode = data.ReasonCode
			first = false
		} else {
			if out.Outcome.Outcome != data.Outcome.Outcome {
				out.Outcome.Outcome = "unknown"
			}
			if out.ReasonCode != data.ReasonCode {
				out.ReasonCode = "replica_results_diverge"
			}
		}
		if data.Outcome.Execution != "completed" {
			out.Outcome.Execution = "execution_failed"
		}
		if out.ValidUntil.IsZero() || (!data.ValidUntil.IsZero() && data.ValidUntil.Before(out.ValidUntil)) {
			out.ValidUntil = data.ValidUntil
		}
	}
	if out.Outcome.Execution != "completed" {
		out.Outcome.Outcome = "unknown"
	}
	return out
}

// Inheritance is resolved against the Monitor revision at execution, not by
// rewriting a reusable profile's source or its independent catalog hash.
func InheritPlacements(plan monitor.Plan, p Profile) Profile {
	p.Actions = append([]Action(nil), p.Actions...)
	for i := range p.Actions {
		if !p.Actions[i].Placement.Inherit {
			continue
		}
		placement := Placement{Strategy: "leastLoaded"}
		if policy := plan.FixedPlacement; policy != nil {
			placement.Strategy = policy.Strategy
			if placement.Strategy == "" {
				placement.Strategy = "fixed"
			}
			placement.Replicas = policy.Replicas
			placement.DistinctBy = policy.DistinctBy
			candidates := policy.Candidates
			if len(candidates) == 0 {
				candidates = policy.Selected
			}
			placement.Pending = len(candidates) == 0
			for _, slot := range candidates {
				placement.AgentRefs = append(placement.AgentRefs, slot.Agent)
			}
			placement.Zones = append([]string(nil), policy.ScopeZones...)
		}
		p.Actions[i].Placement = placement
	}
	return p
}
