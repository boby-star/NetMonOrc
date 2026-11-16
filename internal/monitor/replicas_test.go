package monitor

import (
	"testing"
	"time"
)

func TestPlannedReplicaVotesAndCoverage(t *testing.T) {
	now := time.Now().UTC()
	p := Plan{Name: "web", Revision: "current", Zones: []string{"office"}, MaxAge: time.Minute, MaxSkew: 10 * time.Second, MinCoverage: 1, HealthyMin: .9, DegradedMin: .5, Checks: []Check{{ID: "http", Type: "http", Weight: 1, Required: true}}}
	slots := []Slot{{Agent: "a", Zone: "office"}, {Agent: "b", Zone: "office"}, {Agent: "c", Zone: "office"}}
	cases := []struct {
		name, mode      string
		outcomes        []string
		state, coverage string
	}{
		{"all positive", "", []string{"success", "success", "success"}, "healthy", "complete"},
		{"all negative decisive", "all", []string{"success", "success", "failure"}, "down", "complete"},
		{"any positive decisive", "any", []string{"success", "failure", "failure"}, "healthy", "complete"},
		{"quorum positive", "quorum", []string{"success", "success", "failure"}, "healthy", "complete"},
		{"quorum negative", "quorum", []string{"failure", "failure", "success"}, "down", "complete"},
		{"quorum tie", "quorum", []string{"success", "failure"}, "unknown", "complete"},
		{"required incomplete positive quorum", "quorum", []string{"success", "success", ""}, "unknown", "coverage_degraded"},
		{"required negative quorum despite missing", "quorum", []string{"failure", "failure", ""}, "down", "coverage_degraded"},
		{"any missing is not all failed", "any", []string{"failure", "failure", ""}, "unknown", "coverage_degraded"},
		{"all missing blocks success", "all", []string{"success", "success", ""}, "unknown", "coverage_degraded"},
		{"all one negative suffices", "all", []string{"failure", "", ""}, "down", "coverage_degraded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p.SameZoneAggregation = tc.mode
			planned := slots[:len(tc.outcomes)]
			var observations []Observation
			for i, outcome := range tc.outcomes {
				if outcome != "" {
					observations = append(observations, Observation{ID: slots[i].Agent, Agent: slots[i].Agent, Zone: "office", Monitor: p.Name, Revision: p.Revision, Check: "http", StartedAt: now.Add(-time.Second), FinishedAt: now, Execution: "completed", Outcome: outcome})
				}
			}
			// Repeated delivery cannot supply an extra physical vote.
			observations = append(observations, observations...)
			a, err := Evaluate(p, planned, observations, now)
			if err != nil {
				t.Fatal(err)
			}
			if a.State != tc.state || a.Zones[0].Coverage != tc.coverage {
				t.Fatalf("unexpected assessment: %+v", a)
			}
		})
	}
}
func TestQuorumFailureRequiresCoherentPhysicalWitnesses(t *testing.T) {
	now := time.Now().UTC()
	p := Plan{Name: "web", Revision: "r", SameZoneAggregation: "quorum", Zones: []string{"office"}, MaxAge: time.Minute, MaxSkew: 5 * time.Second, MinCoverage: 1, HealthyMin: .9, DegradedMin: .5, Checks: []Check{{ID: "http", Weight: 1, Required: true}}}
	slots := []Slot{{Agent: "a", Zone: "office"}, {Agent: "b", Zone: "office"}, {Agent: "c", Zone: "office"}}
	var observations []Observation
	for i, slot := range slots {
		outcome := "failure"
		at := now.Add(-20 * time.Second)
		if i == 2 {
			outcome = "success"
			at = now
		}
		observations = append(observations, Observation{ID: slot.Agent, Agent: slot.Agent, Zone: slot.Zone, Monitor: p.Name, Revision: p.Revision, Check: "http", StartedAt: at, FinishedAt: at, Execution: "completed", Outcome: outcome})
	}
	a, err := Evaluate(p, slots, observations, now)
	if err != nil || a.State != "down" {
		t.Fatal("independent coherent negative quorum lost", a, err)
	}
	observations[1].StartedAt = now
	observations[1].FinishedAt = now
	a, err = Evaluate(p, slots, observations, now)
	if err != nil || a.State != "unknown" {
		t.Fatal("incoherent negative votes proved failure", a, err)
	}
	latest := observations[0]
	latest.ID = "new"
	latest.StartedAt = now
	latest.FinishedAt = now
	latest.Execution = "execution_failed"
	latest.Outcome = "unknown"
	observations = append(observations, latest)
	a, err = Evaluate(p, slots, observations, now)
	if err != nil || a.State != "unknown" {
		t.Fatal("new execution failure resurrected an older vote", a, err)
	}
}
func TestReplicaCELValuesRemainDeferred(t *testing.T) {
	p := Plan{Name: "web", Revision: "r", Zones: []string{"office"}, MaxAge: time.Minute, MaxSkew: time.Second, MinCoverage: 1, HealthyMin: .9, DegradedMin: .5, Checks: []Check{{ID: "metrics", Type: "http", Weight: 1}}}
	slots := []Slot{{Agent: "a", Zone: "office"}, {Agent: "b", Zone: "office"}}
	for _, expr := range []string{`checks["metrics"].metrics.httpStatus == 200`, `[checks["metrics"]].exists(c,has(c.values.depth))`} {
		p.Rules = []AssessmentRule{{ID: "rule", Priority: 100, When: expr, State: "healthy", Reason: "RULE"}}
		a, err := NewAssessor(p)
		if err != nil {
			t.Fatal(err)
		}
		if err = a.ValidateSources(slots); err == nil {
			t.Fatal("deferred metric/value aggregation admitted", expr)
		}
	}
	p.Rules = []AssessmentRule{{ID: "rule", Priority: 100, When: `checks["metrics"].outcome == "success" && !has(target.labels.metrics)`, State: "healthy", Reason: "RULE"}}
	a, err := NewAssessor(p)
	if err != nil || a.ValidateSources(slots) != nil {
		t.Fatal("outcome or ordinary label falsely rejected", err)
	}
}
