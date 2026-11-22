package diagnostics_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestFixedConditionsCannotReadIndependentSteps(t *testing.T) {
	for _, parallel := range []int{1, 2} {
		t.Run(fmt.Sprint(parallel), func(t *testing.T) {
			check := monitor.Check{Type: "http", Runner: "blackbox", Timeout: time.Second}
			p := d.Profile{Mode: "fixed", Budget: d.Budget{MaxChecks: 2, MaxDuration: 5 * time.Second, MaxParallel: parallel}, Hypotheses: []string{"fault", "other"}, Actions: []d.Action{
				{ID: "root", Check: check, Cost: 1, Expected: map[string]string{"fault": "either", "other": "either"}, Placement: d.Placement{Strategy: "fixed", AgentRefs: []string{"branch"}}},
				{ID: "reader", Check: check, Cost: 1, Expected: map[string]string{"fault": "either", "other": "either"}, Placement: d.Placement{Strategy: "fixed", AgentRefs: []string{"dc"}}},
			}, Steps: []d.Step{{ID: "a-root", ActionID: "root"}, {ID: "b-reader", ActionID: "reader", When: func(_ context.Context, tasks map[string]d.Outcome, _ string) (bool, error) {
				if _, exists := tasks["a-root"]; exists {
					t.Error("independent result leaked into condition")
				}
				return false, fmt.Errorf("missing predecessor context")
			}}}}
			engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
				if a.ID != "root" {
					t.Error("reader dispatched despite missing context")
				}
				return d.Outcome{Execution: "completed", Outcome: "success"}, nil
			})}
			result, err := engine.Run(context.Background(), p, "fixed", agents(), &d.Ledger{}, nil, "down")
			if err != nil || result.Status != "inconclusive" || result.Attempts != 1 || len(result.BlockedSteps) != 1 || result.BlockedSteps[0] != "b-reader" {
				t.Fatalf("context isolation lost: %+v %v", result, err)
			}
		})
	}
}
