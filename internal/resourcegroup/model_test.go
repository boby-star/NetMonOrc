package resourcegroup

import (
	"testing"
	"time"
)

func TestCriticalGroupStateAndCoverage(t *testing.T) {
	now := time.Now()
	ids := []string{"a", "b"}
	for _, tc := range []struct {
		strategy, a, b string
		min, threshold float64
		want           string
	}{
		{"all-required", "healthy", "unknown", 1, -1, "unknown"}, {"any", "healthy", "unknown", 1, -1, "unknown"},
		{"any", "healthy", "unknown", .5, -1, "healthy"}, {"all-required", "down", "unknown", .5, -1, "degraded"},
		{"all-required", "healthy", "down", 1, -1, "degraded"}, {"any", "healthy", "down", 1, -1, "healthy"},
		{"weighted", "healthy", "down", 1, -1, "degraded"}, {"weighted", "healthy", "down", 1, .9, "healthy"},
		{"weighted", "down", "down", 1, -1, "down"}, {"weighted", "unknown", "unknown", 0, -1, "unknown"},
	} {
		s := Spec{Mode: "derived", Members: &ids, Evaluation: &Evaluation{Strategy: tc.strategy, MinCoverage: &tc.min}}
		if tc.strategy == "weighted" {
			s.Evaluation.Weights = map[string]float64{"a": .95, "b": .05}
			if tc.threshold >= 0 {
				s.Evaluation.HealthyMin = &tc.threshold
			}
		}
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
		src := map[string]Source{"a": {Member: Member{State: tc.a, CoverageState: "sufficient"}, At: now}, "b": {Member: Member{State: tc.b, CoverageState: "sufficient"}, At: now}}
		a := Evaluate("g", s, ids, src, now)
		if a.State == nil || *a.State != tc.want {
			t.Fatalf("%+v got %+v", tc, a)
		}
		if tc.b == "unknown" && a.CoverageState == "sufficient" {
			t.Fatal("missing member covered")
		}
	}
	s := Spec{Members: &ids, Mode: "derived", Evaluation: &Evaluation{Strategy: "any"}}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{now.Add(-6 * time.Second), now.Add(time.Second)} {
		a := Evaluate("g", s, ids, map[string]Source{"a": {Member: Member{State: "healthy"}, At: at}}, now)
		if *a.State != "unknown" {
			t.Fatal("stale/future healthy", a)
		}
	}
	s.Mode = "context"
	s.Evaluation = nil
	a := Evaluate("g", s, nil, nil, now)
	if a.State != nil || a.CoverageState != "unknown" {
		t.Fatal("context invented health")
	}
}
func TestSelectorMissingKeysAndOperators(t *testing.T) {
	s := Selector{MatchExpressions: []Expression{{Key: "env", Operator: "NotIn", Values: []string{"dev"}}}}
	if !s.Match(nil) || s.Match(map[string]string{"env": "dev"}) {
		t.Fatal("NotIn semantics")
	}
	s.MatchExpressions[0].Operator = "In"
	if s.Match(nil) || !s.Match(map[string]string{"env": "dev"}) {
		t.Fatal("In semantics")
	}
}
