package diagnostics_test

import (
	"context"
	"testing"
	"time"

	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
)

func TestExplicitTechnicalRetryAcrossModes(t *testing.T) {
	for _, mode := range []string{"static", "fixed", "adaptive"} {
		t.Run(mode, func(t *testing.T) {
			p := repeatProfile()
			p.Confirmation = nil
			p.ContinueAfterUnique = false
			p.Actions[0].Retry = &d.Retry{MaxAttempts: 2}
			if mode == "fixed" {
				p.Mode = "fixed"
				p.Steps = []d.Step{{ID: "verify", ActionID: "verify"}}
			}
			engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
				if a.AttemptNo == 1 {
					return d.Outcome{Execution: "execution_failed", Outcome: "unknown"}, nil
				}
				return d.Outcome{Execution: "completed", Outcome: "failure"}, nil
			})}
			r, err := engine.Run(context.Background(), p, mode, independentAgents(), &d.Ledger{}, nil, "down")
			if err != nil || r.Status != "completed" || r.Attempts != 2 || r.Cost != 2 || r.Hypotheses[0] != "fault" {
				t.Fatalf("%+v %v", r, err)
			}
			if r.Decisions[0].AttemptNo != 1 || r.Decisions[1].AttemptNo != 2 || len(r.Decisions[1].HypothesesBefore) != 2 || r.Decisions[0].AgentID != r.Decisions[1].AgentID {
				t.Fatal("retry changed executor or filtered on execution error", r)
			}
		})
	}
}

func TestRetryCannotBypassBudgetsOrRepeatUnknown(t *testing.T) {
	for _, tc := range []struct {
		name, outcome string
		limit         int
		retry         bool
		want          int
	}{
		{"default", "execution_failed", 3, false, 1},
		{"exhaustion", "execution_failed", 3, true, 2},
		{"budget", "execution_failed", 1, true, 1},
		{"negative", "failure", 3, true, 1},
		{"unknown", "unknown", 3, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := repeatProfile()
			p.Confirmation = nil
			p.ContinueAfterUnique = false
			p.Budget.MaxChecks = tc.limit
			if tc.retry {
				p.Actions[0].Retry = &d.Retry{MaxAttempts: 2}
			}
			engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
				if tc.outcome == "execution_failed" {
					return d.Outcome{Execution: tc.outcome, Outcome: "unknown"}, nil
				}
				return d.Outcome{Execution: "completed", Outcome: tc.outcome}, nil
			})}
			ledger := &d.Ledger{}
			r, err := engine.Run(context.Background(), p, "adaptive", independentAgents(), ledger, nil, "down")
			if err != nil || r.Attempts != tc.want || ledger.UsedChecks != tc.want {
				t.Fatalf("%+v %v", r, err)
			}
			if tc.outcome == "execution_failed" && (r.Status != "inconclusive" || len(r.Hypotheses) != 2) {
				t.Fatal("technical error became service evidence", r)
			}
			again, err := engine.Run(context.Background(), p, "adaptive", independentAgents(), ledger, nil, "down")
			if err != nil || again.Attempts != 0 {
				t.Fatal("new Run replenished retry allowance", again, err)
			}
		})
	}
}

func TestRetryRetainsConfirmationAndDeadlineLimits(t *testing.T) {
	p := repeatProfile()
	p.Actions[0].Retry = &d.Retry{MaxAttempts: 2}
	p.Actions[0].Placement.Strategy = "roundRobin"
	p.Budget.MaxChecks = 4
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
		if a.AttemptNo == 1 {
			return d.Outcome{Execution: "execution_failed", Outcome: "unknown"}, nil
		}
		return d.Outcome{Execution: "completed", Outcome: "failure"}, nil
	})}
	r, err := engine.Run(context.Background(), p, "adaptive", independentAgents(), &d.Ledger{}, nil, "down")
	if err != nil || r.Status != "completed" || r.Attempts != 4 || r.Confirmation.Observations != 2 || r.Confirmation.DistinctFailureDomains != 2 {
		t.Fatalf("retry counted as confirmation proof: %+v %v", r, err)
	}
	p.Confirmation = nil
	p.ContinueAfterUnique = false
	p.Budget.MaxDuration = time.Second
	engine.Runner = runFunc(func(ctx context.Context, _ d.Action, _ d.Executor) (d.Outcome, error) {
		<-ctx.Done()
		return d.Outcome{}, ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	r, err = engine.Run(ctx, p, "adaptive", independentAgents(), &d.Ledger{}, nil, "down")
	if err != nil || r.Attempts != 1 || r.Reason != "deadline_or_cancelled" {
		t.Fatal("cancelled retry dispatched", r, err)
	}
}
