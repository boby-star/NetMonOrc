package monitor

import (
	"fmt"
	"sort"
	"time"
)

// Evaluate aggregates independently planned physical sources within each zone.
// It accepts only the newest execution per slot: a newer execution failure
// must not silently resurrect an older success.
func Evaluate(p Plan, slots []Slot, history []Observation, now time.Time) (Assessment, error) {
	assessor, e := NewAssessor(p)
	if e != nil {
		return Assessment{}, e
	}
	return assessor.Evaluate(slots, history, now)
}
func (assessor *Assessor) Evaluate(slots []Slot, history []Observation, now time.Time) (Assessment, error) {
	p := assessor.plan
	a := Assessment{Monitor: p.Name, Revision: p.Revision, At: now, State: "unknown"}
	if p.Disabled {
		for _, zone := range p.Zones {
			a.Zones = append(a.Zones, ZoneAssessment{Zone: zone, State: "unknown", Coverage: "coverage_degraded", Evidence: []string{}, Reason: "monitor_disabled"})
		}
		return a, nil
	}
	if err := assessor.ValidateSources(slots); err != nil {
		return a, err
	}
	byZone := map[string][]Slot{}
	agents := map[string]bool{}
	allowed := map[string]bool{}
	for _, zone := range p.Zones {
		allowed[zone] = true
	}
	for _, slot := range slots {
		if slot.Agent == "" || !allowed[slot.Zone] || agents[slot.Agent] {
			return a, fmt.Errorf("invalid or duplicate planned slot %q/%q", slot.Agent, slot.Zone)
		}
		byZone[slot.Zone] = append(byZone[slot.Zone], slot)
		agents[slot.Agent] = true
	}
	checks := map[string]Check{}
	for _, check := range p.Checks {
		checks[check.ID] = check
	}
	for _, zone := range p.Zones {
		z := ZoneAssessment{Zone: zone, State: "unknown", Coverage: "coverage_degraded", Evidence: []string{}, Reason: "insufficient_evidence"}
		latestPhysical := map[string]Observation{}
		expected := map[string]bool{}
		for _, slot := range byZone[zone] {
			expected[slot.Agent] = true
		}
		var anchor time.Time
		for _, o := range history {
			check, known := checks[o.Check]
			age := p.MaxAge
			if check.MaxAge > 0 {
				age = check.MaxAge
			}
			if o.DiagnosticTaskID != "" || !known || o.Monitor != p.Name || o.Revision != p.Revision || o.Zone != zone || !expected[o.Agent] || o.ID == "" || o.FinishedAt.Before(o.StartedAt) || o.FinishedAt.After(now) || now.Sub(o.FinishedAt) > age {
				continue
			}
			key := o.Agent + "/" + o.Check
			old, exists := latestPhysical[key]
			if !exists || o.FinishedAt.After(old.FinishedAt) || (o.FinishedAt.Equal(old.FinishedAt) && o.ID > old.ID) {
				latestPhysical[key] = o
			}
		}
		for _, o := range latestPhysical {
			if anchor.IsZero() || o.FinishedAt.After(anchor) {
				anchor = o.FinishedAt
			}
		}
		latest := map[string]CheckView{}
		evidence := map[string]bool{}
		total, valid, success := 0.0, 0.0, 0.0
		requiredMissing, requiredFailure, divergent := false, false, false
		complete := len(byZone[zone]) > 0
		for _, check := range p.Checks {
			total += check.Weight
			var fresh, current []Observation
			for _, slot := range byZone[zone] {
				if o, found := latestPhysical[slot.Agent+"/"+check.ID]; found {
					fresh = append(fresh, o)
					if !o.FinishedAt.Before(anchor.Add(-p.MaxSkew)) {
						current = append(current, o)
					}
				}
			}
			n := len(byZone[zone])
			outcome := aggregateReplicas(p.SameZoneAggregation, n, current)
			successes, failures, present := 0, 0, 0
			for _, o := range current {
				if validOutcome(o) {
					present++
					evidence[o.ID] = true
					if o.Outcome == "success" {
						successes++
					} else {
						failures++
					}
				}
			}
			if successes > 0 && failures > 0 {
				divergent = true
			}
			physicalComplete := n > 0 && present == n
			complete = complete && physicalComplete
			if check.Required && !physicalComplete {
				requiredMissing = true
			}
			if check.Required {
				proof := failureWitness(p.SameZoneAggregation, n, fresh, p.MaxSkew)
				if len(proof) > 0 {
					requiredFailure = true
					outcome = "failure"
					for _, o := range proof {
						evidence[o.ID] = true
					}
				}
			}
			if outcome == "unknown" {
				if check.Required {
					requiredMissing = true
				}
			} else {
				valid += check.Weight
				if outcome == "success" {
					success += check.Weight
				}
			}
			if n == 1 && len(current) == 1 {
				latest[check.ID] = assessmentCheckView(current[0], check.Type, now)
			} else if len(current) > 0 {
				at := current[0].FinishedAt
				for _, o := range current {
					if o.FinishedAt.Before(at) {
						at = o.FinishedAt
					}
				}
				view := assessmentCheckView(Observation{FinishedAt: at, Reason: "replica_aggregate"}, check.Type, now)
				view.Outcome = outcome
				view.ExecutionStatus = "completed"
				if len(current) < n {
					view.ExecutionStatus = "not_executed"
				}
				for _, o := range current {
					if o.Execution != "completed" {
						view.ExecutionStatus = "failed"
					}
				}
				if successes > 0 && failures > 0 {
					view.ReasonCode = "AGENT_RESULTS_DIVERGE"
				}
				latest[check.ID] = view
			}
		}
		for id := range evidence {
			z.Evidence = append(z.Evidence, id)
		}
		sort.Strings(z.Evidence)
		if total > 0 {
			z.Q = valid / total
		}
		if valid > 0 {
			h := success / valid
			z.H = &h
		}
		coherent := len(z.Evidence) > 0
		if complete && coherent {
			z.Coverage = "complete"
		}
		switch {
		case requiredFailure:
			z.State = "down"
			z.Reason = "required_failure"
		case !coherent:
			z.Reason = "missing_or_incoherent_evidence"
		case requiredMissing || z.Q < p.MinCoverage || z.H == nil:
		case p.WeightedDisabled:
			z.State = p.DefaultState
			if z.State == "" {
				z.State = "unknown"
			}
			z.Reason = "default_state"
		case *z.H < p.DegradedMin:
			z.State = "down"
			z.Reason = "weighted_failure"
		case *z.H < p.HealthyMin:
			z.State = "degraded"
			z.Reason = "weighted_degradation"
		default:
			z.State = "healthy"
			z.Reason = "sufficient_success"
		}
		assessor.applyRules(&z, latest, now, coherent, requiredFailure, requiredMissing)
		if divergent {
			z.Reason = "AGENT_RESULTS_DIVERGE"
		}
		a.Zones = append(a.Zones, z)
	}
	a.State = aggregateZones(p.ZoneAggregation, a.Zones)
	return a, nil
}

// A local failure proves degradation even when another zone is unknown. It
// proves a global outage only when every required zone is down. Explicit any
// is an availability SLA, not majority voting; zone evidence stays untouched.
func aggregateZones(policy string, zones []ZoneAssessment) string {
	if len(zones) == 0 {
		return "unknown"
	}
	counts := map[string]int{}
	for _, zone := range zones {
		counts[zone.State]++
	}
	switch {
	case policy == "any" && counts["healthy"] > 0:
		return "healthy"
	case counts["healthy"] == len(zones):
		return "healthy"
	case counts["down"] == len(zones):
		return "down"
	case counts["degraded"] > 0 || counts["down"] > 0:
		return "degraded"
	default:
		return "unknown"
	}
}
