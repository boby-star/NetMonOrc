package diagnostics_test

import (
	"context"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"testing"
	"time"
)

type admittingRunner struct {
	admit func(context.Context, d.Action, d.Executor) (d.Runner, func(), error)
}

func (r admittingRunner) Admit(ctx context.Context, a d.Action, e d.Executor) (d.Runner, func(), error) {
	return r.admit(ctx, a, e)
}
func (r admittingRunner) Execute(context.Context, d.Action, d.Executor) (d.Outcome, error) {
	panic("unadmitted execution")
}
func TestAdmissionCancellationDoesNotSpendAnyBudget(t *testing.T) {
	for _, mode := range []string{"static", "fixed", "adaptive"} {
		t.Run(mode, func(t *testing.T) {
			p := repeatProfile()
			p.Confirmation = nil
			p.ContinueAfterUnique = false
			if mode == "fixed" {
				p.Mode = "fixed"
				p.Steps = []d.Step{{ID: "verify", ActionID: "verify"}}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runner := admittingRunner{admit: func(wait context.Context, _ d.Action, _ d.Executor) (d.Runner, func(), error) {
				cancel()
				<-wait.Done()
				return nil, nil, wait.Err()
			}}
			ledger := &d.Ledger{}
			r, err := (d.Engine{Runner: runner, Eligible: gate}).Run(ctx, p, mode, independentAgents(), ledger, nil, "down")
			if err != nil || r.Status != "inconclusive" || r.Attempts != 0 || r.Cost != 0 || ledger.UsedChecks != 0 || len(r.Decisions) != 0 {
				t.Fatalf("waiting became a physical attempt: %+v %v", r, err)
			}
		})
	}
}
func TestAdmissionWaitRefreshesExpiredBaselineBeforeDispatch(t *testing.T) {
	p := repeatProfile()
	p.Confirmation.MinDistinctFailureDomains = 1
	expires := time.Now().Add(50 * time.Millisecond)
	proof := d.Evidence{ActionID: "verify", AgentID: "branch", ObservationID: "expires", ValidUntil: expires, Outcome: d.Outcome{Execution: "completed", Outcome: "failure"}}
	admissions, releases := 0, 0
	runner := admittingRunner{admit: func(ctx context.Context, _ d.Action, _ d.Executor) (d.Runner, func(), error) {
		admissions++
		if admissions == 1 {
			timer := time.NewTimer(time.Until(expires) + time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-timer.C:
			}
		}
		return runFunc(failureRunner), func() { releases++ }, nil
	}}
	ledger := &d.Ledger{}
	// A full telemetry snapshot must not discard a future admissible pair:
	// the shared reservation, not this stale gauge, controls dispatch.
	sources := independentAgents()
	for i := range sources {
		sources[i].Active = sources[i].Capacity
	}
	r, err := (d.Engine{Runner: runner, Eligible: gate}).Run(context.Background(), p, "adaptive", sources, ledger, []d.Evidence{proof}, "down")
	if err != nil || r.Status != "completed" || r.Attempts != 2 || admissions != 3 || releases != 3 || ledger.UsedChecks != 2 {
		t.Fatalf("expired baseline was not replanned after admission: %+v admissions=%d releases=%d %v", r, admissions, releases, err)
	}
	for _, id := range r.Confirmation.CountedEvidence {
		if id == "observation:expires" {
			t.Fatal("expired queued proof counted")
		}
	}
}
