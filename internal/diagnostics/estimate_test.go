package diagnostics_test

import (
	"context"
	"testing"

	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
)

func TestLiveEstimatedBudgetIncludesEveryRetry(t *testing.T) {
	p := repeatProfile()
	p.Confirmation = nil
	p.ContinueAfterUnique = false
	p.Actions[0].Check.MaxBodyBytes = 16384
	p.Actions[0].Retry = &d.Retry{MaxAttempts: 3}
	n, known := d.EstimateCheckBytes(p.Actions[0].Check)
	if !known || n <= 16384 {
		t.Fatal("missing bounded-body and protocol estimate")
	}
	p.Budget.MaxEstimatedBytes = n
	if err := d.ValidateLiveProfile(p); err != nil {
		t.Fatal(err)
	}
	engine := d.Engine{Eligible: gate, EstimateBytes: d.EstimateBytes, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		return d.Outcome{Execution: "execution_failed", Outcome: "unknown"}, nil
	})}
	ledger := &d.Ledger{}
	r, err := engine.Run(context.Background(), p, "adaptive", independentAgents(), ledger, nil, "down")
	if err != nil || r.Attempts != 1 || r.Reason != "estimated_byte_budget_exhausted" || r.EstimatedBytes == nil || *r.EstimatedBytes != n {
		t.Fatalf("retry exceeded estimated budget: %+v %v", r, err)
	}
	r, err = engine.Run(context.Background(), p, "adaptive", independentAgents(), ledger, nil, "down")
	if err != nil || r.Attempts != 0 || ledger.EstimatedBytes != n {
		t.Fatal("rerun replenished byte budget", r, err)
	}
	p.Actions[0].Check.FollowRedirects = true
	redirect, _ := d.EstimateCheckBytes(p.Actions[0].Check)
	if redirect != 10*n {
		t.Fatal("redirect estimate ignores additional hops")
	}
	p.Actions[0].Check.Type = "unknown"
	if err = d.ValidateLiveProfile(p); err == nil {
		t.Fatal("unknown estimate accepted in live budget")
	}
}
