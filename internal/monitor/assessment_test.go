package monitor

import (
	"testing"
	"time"
)

func TestEvidenceAndZoneInvariants(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	p := Plan{Name: "web", Revision: "current", Zones: []string{"office", "server"}, MaxAge: time.Minute, MaxSkew: 10 * time.Second, MinCoverage: 1, HealthyMin: .9, DegradedMin: .5, Checks: []Check{{ID: "ping", Weight: .95}, {ID: "http", Weight: .05, Required: true}}}
	slots := []Slot{{Agent: "a", Zone: "office"}, {Agent: "b", Zone: "server"}}
	var baseline []Observation
	for _, s := range slots {
		for _, c := range p.Checks {
			baseline = append(baseline, Observation{ID: s.Agent + c.ID, Monitor: p.Name, Revision: p.Revision, Check: c.ID, Agent: s.Agent, Zone: s.Zone, StartedAt: now.Add(-time.Second), FinishedAt: now, Execution: "completed", Outcome: "success"})
		}
	}
	for _, tc := range []struct {
		name                     string
		mutate                   func([]Observation) []Observation
		global, office, coverage string
	}{
		{"healthy", func(o []Observation) []Observation { return o }, "healthy", "healthy", "complete"},
		{"required overrides high score", func(o []Observation) []Observation { o[1].Outcome = "failure"; return o }, "degraded", "down", "complete"},
		{"missing required", func(o []Observation) []Observation { return append(o[:1], o[2:]...) }, "unknown", "unknown", "coverage_degraded"},
		{"old revision", func(o []Observation) []Observation { o[1].Revision = "previous"; return o }, "unknown", "unknown", "coverage_degraded"},
		{"expired", func(o []Observation) []Observation {
			o[1].StartedAt = now.Add(-2 * time.Minute)
			o[1].FinishedAt = o[1].StartedAt
			return o
		}, "unknown", "unknown", "coverage_degraded"},
		{"skew", func(o []Observation) []Observation {
			o[1].StartedAt = now.Add(-20 * time.Second)
			o[1].FinishedAt = o[1].StartedAt
			return o
		}, "unknown", "unknown", "coverage_degraded"},
		{"required failure despite skew", func(o []Observation) []Observation {
			o[1].Outcome = "failure"
			o[1].StartedAt = now.Add(-20 * time.Second)
			o[1].FinishedAt = o[1].StartedAt
			return o
		}, "degraded", "down", "coverage_degraded"},
		{"duplicate no additional vote", func(o []Observation) []Observation { return append(o, o...) }, "healthy", "healthy", "complete"},
		{"new execution failure replaces success", func(o []Observation) []Observation {
			o[1].FinishedAt = now.Add(-time.Second)
			n := o[1]
			n.ID = "new"
			n.FinishedAt = now
			n.Execution = "execution_failed"
			n.Outcome = "unknown"
			return append(o, n)
		}, "unknown", "unknown", "coverage_degraded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.mutate(append([]Observation(nil), baseline...))
			a, e := Evaluate(p, slots, o, now)
			if e != nil {
				t.Fatal(e)
			}
			if a.State != tc.global || a.Zones[0].State != tc.office || a.Zones[0].Coverage != tc.coverage {
				t.Fatalf("unexpected assessment: %+v", a)
			}
		})
	}
}
