package diagnostics_test

import (
	"context"
	"fmt"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

type observedFunc func(context.Context, d.Action, d.Executor) (d.ObservedOutcome, error)

func (f observedFunc) ExecuteEvidence(c context.Context, a d.Action, e d.Executor) (d.ObservedOutcome, error) {
	return f(c, a, e)
}
func (f observedFunc) Execute(c context.Context, a d.Action, e d.Executor) (d.Outcome, error) {
	r, err := f(c, a, e)
	return r.Outcome, err
}
func temporalProfile() d.Profile {
	return d.Profile{Mode: "fixed", Hypotheses: []string{"fault", "other"}, Budget: d.Budget{MaxChecks: 2, MaxDuration: time.Second, MaxParallel: 1}, Actions: []d.Action{
		{ID: "a", Placement: d.Placement{Strategy: "leastLoaded"}, Cost: 1, Check: monitor.Check{Type: "http", Runner: "blackbox", Timeout: 100 * time.Millisecond}, Expected: map[string]string{"fault": "failure", "other": "success"}},
		{ID: "b", Placement: d.Placement{Strategy: "leastLoaded"}, Cost: 1, Check: monitor.Check{Type: "http", Runner: "blackbox", Timeout: 100 * time.Millisecond}, Expected: map[string]string{"fault": "either", "other": "success"}},
	}, Steps: []d.Step{{ID: "a", ActionID: "a"}, {ID: "b", ActionID: "b", DependsOn: []string{"a"}}}}
}
func TestTemporalWindowRebuildsHypothesesWithoutRefund(t *testing.T) {
	for _, tc := range []struct {
		name      string
		age, skew time.Duration
		state     string
	}{
		{"coherent", time.Second, time.Second, "completed"},
		{"maxSkew", time.Second, 5 * time.Millisecond, "inconclusive"},
		{"TTL", 5 * time.Millisecond, time.Second, "inconclusive"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := observedFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.ObservedOutcome, error) {
				outcome := "failure"
				if a.ID == "b" {
					time.Sleep(20 * time.Millisecond)
					outcome = "success"
				}
				return d.ObservedOutcome{ObservationID: a.ID, FinishedAt: time.Now(), Outcome: d.Outcome{Execution: "completed", Outcome: outcome}}, nil
			})
			ledger := &d.Ledger{}
			r, err := (d.Engine{Runner: runner, Eligible: gate, EvidenceMaxAge: tc.age, EvidenceMaxSkew: tc.skew}).Run(context.Background(), temporalProfile(), "fixed", independentAgents(), ledger, nil, "down")
			if err != nil || r.Status != tc.state || r.Attempts != 2 || ledger.UsedChecks != 2 || r.Cost != 2 {
				t.Fatalf("time window/budget incorrect: %+v %v", r, err)
			}
			if tc.state == "inconclusive" && len(r.Hypotheses) != 2 {
				t.Fatal("stale elimination survived", r.Hypotheses)
			}
			if r.Decisions[0].FinishedAt == nil || r.Decisions[0].ValidUntil == nil || r.EvidencePolicy == nil {
				t.Fatal("physical evidence timing not retained")
			}
		})
	}
}
func TestLatestRepeatedResultReplacesOldElimination(t *testing.T) {
	p := repeatProfile()
	p.Confirmation.MinDistinctFailureDomains = 1
	calls := 0
	runner := observedFunc(func(context.Context, d.Action, d.Executor) (d.ObservedOutcome, error) {
		calls++
		outcome := "success"
		if calls == 1 {
			outcome = "failure"
		}
		return d.ObservedOutcome{ObservationID: fmt.Sprint(calls), FinishedAt: time.Now(), Outcome: d.Outcome{Execution: "completed", Outcome: outcome}}, nil
	})
	ledger := &d.Ledger{}
	r, err := (d.Engine{Runner: runner, Eligible: gate, EvidenceMaxAge: time.Second, EvidenceMaxSkew: time.Second}).Run(context.Background(), p, "adaptive", independentAgents(), ledger, nil, "down")
	if err != nil || r.Status != "completed" || len(r.Hypotheses) != 1 || r.Hypotheses[0] != "other" || r.Attempts != 3 || ledger.UsedChecks != 3 || r.Confirmation.Observations != 2 {
		t.Fatalf("old same-pair proof created false conflict: %+v %v", r, err)
	}
}
func TestDelayedOrFutureEvidenceDoesNotBecomeFreshAtReceipt(t *testing.T) {
	for _, offset := range []time.Duration{-time.Second, time.Minute} {
		t.Run(offset.String(), func(t *testing.T) {
			p := temporalProfile()
			p.Actions = p.Actions[:1]
			p.Steps = p.Steps[:1]
			runner := observedFunc(func(context.Context, d.Action, d.Executor) (d.ObservedOutcome, error) {
				return d.ObservedOutcome{ObservationID: "delayed", FinishedAt: time.Now().Add(offset), Outcome: d.Outcome{Execution: "completed", Outcome: "failure"}}, nil
			})
			r, err := (d.Engine{Runner: runner, Eligible: gate, EvidenceMaxAge: 100 * time.Millisecond, EvidenceMaxSkew: time.Second}).Run(context.Background(), p, "fixed", independentAgents(), &d.Ledger{}, nil, "down")
			if err != nil || r.Status != "inconclusive" || len(r.Hypotheses) != 2 || r.Attempts != 1 || r.Decisions[0].Outcome.Execution != "completed" {
				t.Fatalf("delivery time refreshed or mislabelled proof: %+v %v", r, err)
			}
		})
	}
}

func TestFreshDiagnosticCannotConfirmOldBaselineOutsideMaxSkew(t *testing.T) {
	p := temporalProfile()
	p.Mode = "adaptive"
	p.Steps = nil
	p.Budget.MaxChecks = 1
	p.Confirmation = &d.Confirmation{ActionRefs: []string{"a", "b"}, MinObservations: 2, MinDistinctFailureDomains: 1}
	p.Actions[1].Expected = map[string]string{"fault": "success", "other": "success"}
	now := time.Now()
	baseline := d.Evidence{ActionID: "a", AgentID: "branch", ObservationID: "baseline", FinishedAt: now.Add(-20 * time.Second), ValidUntil: now.Add(time.Minute), Outcome: d.Outcome{Execution: "completed", Outcome: "failure"}}
	runner := observedFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.ObservedOutcome, error) {
		if a.ID != "b" {
			t.Fatal("unexpected confirmation action", a.ID)
		}
		return d.ObservedOutcome{ObservationID: "fresh-b", FinishedAt: time.Now(), Outcome: d.Outcome{Execution: "completed", Outcome: "success"}}, nil
	})
	ledger := &d.Ledger{}
	r, err := (d.Engine{Runner: runner, Eligible: gate, EvidenceMaxAge: time.Minute, EvidenceMaxSkew: 5 * time.Second}).Run(context.Background(), p, "adaptive", independentAgents(), ledger, []d.Evidence{baseline}, "down")
	if err != nil || r.Status != "inconclusive" || len(r.Hypotheses) != 2 || r.Attempts != 1 || ledger.UsedChecks != 1 || r.Confirmation.Satisfied || r.Confirmation.Observations != 0 {
		t.Fatalf("temporally unrelated baseline confirmed a class: %+v %v", r, err)
	}
}

func TestPartialRetryRetainsLatestTechnicalFailureInWorkingSet(t *testing.T) {
	p := repeatProfile()
	p.Budget.MaxChecks = 1
	p.Confirmation.MinDistinctFailureDomains = 1
	p.Actions[0].Retry = &d.Retry{MaxAttempts: 2}
	now := time.Now()
	baseline := d.Evidence{ActionID: "verify", AgentID: "branch", ObservationID: "old-failure", FinishedAt: now, ValidUntil: now.Add(time.Second), Outcome: d.Outcome{Execution: "completed", Outcome: "failure"}}
	runner := observedFunc(func(context.Context, d.Action, d.Executor) (d.ObservedOutcome, error) {
		return d.ObservedOutcome{ObservationID: "technical", FinishedAt: time.Now(), Outcome: d.Outcome{Execution: "execution_failed", Outcome: "unknown"}}, nil
	})
	ledger := &d.Ledger{}
	r, err := (d.Engine{Runner: runner, Eligible: gate, EvidenceMaxAge: time.Second, EvidenceMaxSkew: time.Second}).Run(context.Background(), p, "adaptive", independentAgents(), ledger, []d.Evidence{baseline}, "down")
	if err != nil || r.Reason != "check_budget_exhausted" || r.Status != "inconclusive" || len(r.Hypotheses) != 2 || r.Attempts != 1 || ledger.UsedChecks != 1 {
		t.Fatalf("partial retry resurrected an older result: %+v %v", r, err)
	}
}
