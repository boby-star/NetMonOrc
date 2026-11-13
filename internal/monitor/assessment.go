package monitor

import (
	"fmt"
	"time"
)

// Evaluate supports one planned agent per zone in this first research slice.
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
	byZone := map[string]string{}
	agents := map[string]bool{}
	allowed := map[string]bool{}
	for _, z := range p.Zones {
		allowed[z] = true
	}
	for _, s := range slots {
		if s.Agent == "" || !allowed[s.Zone] || byZone[s.Zone] != "" || agents[s.Agent] {
			return a, fmt.Errorf("invalid or duplicate planned slot %q/%q", s.Agent, s.Zone)
		}
		byZone[s.Zone] = s.Agent
		agents[s.Agent] = true
	}
	for _, zone := range p.Zones {
		z := ZoneAssessment{Zone: zone, State: "unknown", Coverage: "coverage_degraded", Evidence: []string{}, Reason: "insufficient_evidence"}
		latest := map[string]Observation{}
		for _, o := range history {
			age := p.MaxAge
			known := false
			for _, c := range p.Checks {
				if c.ID == o.Check {
					known = true
					if c.MaxAge > 0 {
						age = c.MaxAge
					}
					break
				}
			}
			if o.DiagnosticTaskID != "" || !known || o.Monitor != p.Name || o.Revision != p.Revision || o.Zone != zone || byZone[zone] == "" || o.Agent != byZone[zone] || o.ID == "" || o.FinishedAt.Before(o.StartedAt) || o.FinishedAt.After(now) || now.Sub(o.FinishedAt) > age {
				continue
			}
			old, ok := latest[o.Check]
			if !ok || o.FinishedAt.After(old.FinishedAt) || (o.FinishedAt.Equal(old.FinishedAt) && o.ID > old.ID) {
				latest[o.Check] = o
			}
		}
		var total, valid, success float64
		requiredMissing, requiredFailure := false, false
		var earliest, newest time.Time
		for _, c := range p.Checks {
			total += c.Weight
			o, ok := latest[c.ID]
			ok = ok && o.Execution == "completed" && (o.Outcome == "success" || o.Outcome == "failure")
			if !ok {
				if c.Required {
					requiredMissing = true
				}
				continue
			}
			valid += c.Weight
			if o.Outcome == "success" {
				success += c.Weight
			}
			if c.Required && o.Outcome == "failure" {
				requiredFailure = true
			}
			z.Evidence = append(z.Evidence, o.ID)
			if earliest.IsZero() || o.FinishedAt.Before(earliest) {
				earliest = o.FinishedAt
			}
			if newest.IsZero() || o.FinishedAt.After(newest) {
				newest = o.FinishedAt
			}
		}
		if total > 0 {
			z.Q = valid / total
		}
		if valid > 0 {
			h := success / valid
			z.H = &h
		}
		// Coverage counts physical slots separately from weighted coverage:
		// a missing zero-weight check still means incomplete observations.
		complete := byZone[zone] != ""
		for _, c := range p.Checks {
			o, ok := latest[c.ID]
			if !ok || o.Execution != "completed" || (o.Outcome != "success" && o.Outcome != "failure") {
				complete = false
			}
		}
		coherent := !earliest.IsZero() && newest.Sub(earliest) <= p.MaxSkew
		if complete && coherent {
			z.Coverage = "complete"
		}
		switch {
		case requiredFailure:
			z.State = "down"
			z.Reason = "required_failure"
		case !coherent:
			z.Reason = "missing_or_incoherent_evidence"
		case requiredMissing || z.Q < p.MinCoverage || z.H == nil: // unknown
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
		a.Zones = append(a.Zones, z)
	}
	counts := map[string]int{}
	for _, z := range a.Zones {
		counts[z.State]++
	}
	switch {
	case counts["healthy"] == len(p.Zones):
		a.State = "healthy"
	case counts["down"] == len(p.Zones):
		a.State = "down"
	case counts["degraded"] > 0 || (counts["down"] > 0 && counts["healthy"] > 0):
		a.State = "degraded"
	}
	return a, nil
}
