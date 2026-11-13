package monitor

import (
	"testing"
	"time"
)

func TestCELAssessmentSafety(t *testing.T) {
	now := time.Now().UTC()
	p := Plan{Name: "web", Revision: "current", TargetAddress: "127.0.0.1", Zones: []string{"office"}, MaxAge: time.Minute, MaxSkew: time.Second, MinCoverage: 1, HealthyMin: .9, DegradedMin: .5, Checks: []Check{{ID: "http", Type: "http", Weight: 1, Required: true}}}
	o := Observation{ID: "one", Monitor: p.Name, Revision: p.Revision, Check: "http", Agent: "branch", Zone: "office", StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "success", StatusCode: 200}
	slots := []Slot{{Agent: "branch", Zone: "office"}}
	for _, tc := range []struct {
		name    string
		rules   []AssessmentRule
		outcome string
		state   string
		errors  int
	}{
		{"status", []AssessmentRule{{ID: "status", Priority: 100, When: `checks["http"].metrics.httpStatus >= 200`, State: "degraded", Reason: "CUSTOM_STATUS"}}, "success", "degraded", 0},
		{"required cannot be cancelled", []AssessmentRule{{ID: "force-up", Priority: 100, When: `true`, State: "healthy", Reason: "RULE_UP"}}, "failure", "down", 0},
		{"missing metric blocks healthy", []AssessmentRule{{ID: "missing", Priority: 200, When: `checks["http"].metrics.latencyMs > 100`, State: "down", Reason: "QUEUE"}, {ID: "up", Priority: 100, When: `true`, State: "healthy", Reason: "UP"}}, "success", "unknown", 1},
		{"explicit missing guard", []AssessmentRule{{ID: "missing", Priority: 100, When: `!has(checks["http"].metrics.latencyMs)`, State: "unknown", Reason: "NO_VALUE"}}, "success", "unknown", 0},
		{"priority", []AssessmentRule{{ID: "low", Priority: 100, When: `true`, State: "healthy", Reason: "UP"}, {ID: "high", Priority: 200, When: `true`, State: "degraded", Reason: "DEGRADED"}}, "success", "degraded", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.Rules = tc.rules
			o.Outcome = tc.outcome
			a, e := Evaluate(p, slots, []Observation{o}, now)
			if e != nil {
				t.Fatal(e)
			}
			if a.State != tc.state || len(a.Zones[0].RuleErrors) != tc.errors {
				t.Fatalf("unexpected CEL state %+v", a)
			}
		})
	}
	for _, expression := range []string{`zone.state == "healthy"`, `monitor.state == "down"`, `checks["http"].does_not_exist == "x"`, `checks["http"].outcome`} {
		p.Rules = []AssessmentRule{{ID: "bad", Priority: 100, When: expression, State: "down", Reason: "BAD"}}
		if _, e := NewAssessor(p); e == nil {
			t.Fatalf("invalid context/type accepted: %s", expression)
		}
	}
}
