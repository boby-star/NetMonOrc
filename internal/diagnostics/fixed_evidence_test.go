package diagnostics_test

import (
	"context"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

func TestCommittedEvidenceReachesFixedGuardInBothEngines(t *testing.T) {
	for _, parallel := range []int{1, 2} {
		p := parallelProfile()
		p.Budget.MaxParallel = parallel
		called := false
		p.Steps[0].When = nil
		p.Steps[0].WhenEvidence = func(_ context.Context, tasks map[string]d.TaskEvidence, _ string) (bool, error) {
			called = true
			data := tasks["b-left"]
			if data.Values["serial"].Integer != 9007199254740993 || data.ReasonCode != "HTTP_EXPECTED_STATUS" {
				t.Errorf("lost committed data: %+v", data)
			}
			return true, nil
		}
		engine := d.Engine{Eligible: gate, Runner: observedFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.ObservedOutcome, error) {
			return d.ObservedOutcome{ObservationID: a.ID, FinishedAt: time.Now(), Outcome: d.Outcome{Execution: "completed", Outcome: "success"}, Values: map[string]monitor.Scalar{"serial": {Kind: "integer", Integer: 9007199254740993}}, ReasonCode: "HTTP_EXPECTED_STATUS"}, nil
		})}
		result, err := engine.Run(context.Background(), p, "fixed", agents(), &d.Ledger{}, nil, "down")
		if err != nil || !called || result.Attempts != 3 {
			t.Fatalf("guard data lost for parallel=%d: %+v %v", parallel, result, err)
		}
	}
}
