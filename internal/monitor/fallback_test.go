package monitor

import (
	"testing"
	"time"
)

func TestFallbackCoveragePreservesPhysicalIdentityAndDistinctVotes(t *testing.T) {
	now := time.Now()
	roster := []Slot{{"root-a", "office"}, {"root-b", "office"}, {"spare", "office"}, {"remote", "server"}}
	c := Check{ID: "http", Type: "http", Weight: 1, Required: true, MaxAge: time.Minute, Fallback: &FallbackPolicy{On: []string{"runner_unavailable"}, Strategy: "same-zone", MaxAttempts: 2}}
	p := Plan{Name: "service", Revision: "revision", Checks: []Check{c}, Zones: []string{"office"}, MaxAge: time.Minute, MaxSkew: time.Minute, MinCoverage: 1, HealthyMin: 1, DegradedMin: 0.5, FallbackRoster: roster, FixedPlacement: &FixedPlacement{Selected: roster[:1], Roster: roster, Replicas: 1}}
	o := Observation{ID: "fallback", FallbackTaskID: "fallback", FallbackForAgent: "root-a", Monitor: p.Name, Revision: p.Revision, Check: c.ID, Agent: "spare", Zone: "office", StartedAt: now.Add(-time.Second), FinishedAt: now, Execution: "completed", Outcome: "success"}
	a, err := Evaluate(p, roster[:1], []Observation{o}, now)
	if err != nil || a.State != "healthy" || a.Zones[0].Coverage != "complete" {
		t.Fatal("same-zone spare did not restore coverage", a, err)
	}
	if o.Agent != "spare" {
		t.Fatal("physical identity relabelled")
	}
	different := o
	different.Agent = "remote"
	different.Zone = "server"
	different.FallbackForAgent = ""
	a, err = Evaluate(p, roster[:1], []Observation{different}, now)
	if err != nil || a.State != "unknown" {
		t.Fatal("other-zone result supplied original coverage", a, err)
	}
	p.FixedPlacement.Selected = roster[:2]
	p.FixedPlacement.Replicas = 2
	second := o
	second.ID = "second"
	second.FallbackForAgent = "root-b"
	a, err = Evaluate(p, roster[:2], []Observation{o, second}, now)
	if err != nil || a.State != "unknown" || a.Zones[0].Coverage == "complete" {
		t.Fatal("one spare forged two replica votes", a, err)
	}
	for _, reason := range []string{"negative_probe", "target_request_failed", "http_body_mismatch", "target_policy_denied"} {
		bad := o
		bad.DiagnosticTaskID = ""
		bad.FallbackTaskID = ""
		bad.Execution = "completed"
		bad.Outcome = "failure"
		bad.Reason = reason
		if FallbackCause(bad) != "" {
			t.Fatal("target failure triggered fallback", reason)
		}
	}
}
