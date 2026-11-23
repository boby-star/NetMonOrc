package diagnostics_test

import (
	"context"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

func TestFixedReplicasWaitForAllAndKeepValuesSeparate(t *testing.T) {
	for _, parallel := range []int{1, 2} {
		p := parallelProfile()
		p.Budget.MaxParallel = parallel
		p.Budget.MaxChecks = 3
		p.Actions = p.Actions[:1]
		p.Actions[0].Placement = d.Placement{Strategy: "fixed", AgentRefs: []string{"branch", "dc"}, Replicas: 2, DistinctBy: "zone"}
		follow := p.Actions[0]
		follow.ID = "follow"
		follow.Placement = d.Placement{Strategy: "fixed", AgentRefs: []string{"branch"}}
		p.Actions = append(p.Actions, follow)
		called := false
		p.Steps = []d.Step{{ID: "read", ActionID: p.Actions[0].ID}, {ID: "follow", ActionID: "follow", DependsOn: []string{"read"}, WhenContext: func(_ context.Context, tasks map[string]d.TaskEvidence, _ d.FixedContext) (bool, error) {
			called = true
			data := tasks["read"]
			if len(data.ReplicaResults) != 2 || len(data.Values) != 0 || data.ReplicaResults["branch"].Values["depth"].Integer != 0 || data.ReplicaResults["dc"].Values["depth"].Integer != 10 {
				t.Errorf("replica data merged or missing: %+v", data)
			}
			return true, nil
		}}}
		engine := d.Engine{Eligible: gate, Runner: observedFunc(func(_ context.Context, a d.Action, agent d.Executor) (d.ObservedOutcome, error) {
			value := int64(0)
			if agent.ID == "dc" {
				value = 10
			}
			return d.ObservedOutcome{ObservationID: a.ID + agent.ID, FinishedAt: time.Now(), Outcome: d.Outcome{Execution: "completed", Outcome: "success"}, Values: map[string]monitor.Scalar{"depth": {Kind: "integer", Integer: value}}}, nil
		})}
		r, err := engine.Run(context.Background(), p, "fixed", agents(), &d.Ledger{}, nil, "down")
		if err != nil || !called || r.Attempts != 3 || r.Status != "completed" {
			t.Fatalf("parallel=%d incomplete replica set: %+v %v", parallel, r, err)
		}
	}
}
func TestAdaptiveReplicasSpendEveryPhysicalAttempt(t *testing.T) {
	p, _ := profiles(t)
	p.Actions = p.Actions[:1]
	p.Actions[0].Placement = d.Placement{Strategy: "fixed", AgentRefs: []string{"branch", "dc"}, Replicas: 2, DistinctBy: "zone"}
	p.Actions[0].Expected = map[string]string{"fault": "failure", "other": "success"}
	p.Hypotheses = []string{"fault", "other"}
	p.Budget.MaxChecks = 2
	for _, conflict := range []bool{false, true} {
		engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, _ d.Action, agent d.Executor) (d.Outcome, error) {
			outcome := "success"
			if conflict && agent.ID == "dc" {
				outcome = "failure"
			}
			return d.Outcome{Execution: "completed", Outcome: outcome}, nil
		})}
		r, err := engine.Run(context.Background(), p, "adaptive", agents(), &d.Ledger{}, nil, "down")
		expected := "completed"
		if conflict {
			expected = "inconclusive"
		}
		if err != nil || r.Attempts != 2 || r.Status != expected {
			t.Fatalf("replicas not fully accounted: %+v %v", r, err)
		}
	}
	p.Budget.MaxChecks = 1
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		t.Fatal("partial set dispatched without minimum budget")
		return d.Outcome{}, nil
	})}
	r, err := engine.Run(context.Background(), p, "adaptive", agents(), &d.Ledger{}, nil, "down")
	if err != nil || r.Attempts != 0 {
		t.Fatal(r, err)
	}
}

func TestInheritedReplicaPlacementUsesMonitorAndPreservesPending(t *testing.T) {
	p, _ := profiles(t)
	p.Actions = p.Actions[:1]
	p.Hypotheses = []string{"fault", "other"}
	p.Actions[0].Expected = map[string]string{"fault": "failure", "other": "success"}
	p.Actions[0].Placement = d.Placement{Strategy: "leastLoaded", Inherit: true}
	p.Budget.MaxChecks = 2
	plan := monitor.Plan{FixedPlacement: &monitor.FixedPlacement{Strategy: "fixed", Replicas: 2, DistinctBy: "zone", Selected: []monitor.Slot{{Agent: "branch", Zone: "office"}, {Agent: "dc", Zone: "server"}}}}
	engine := d.Engine{CELContext: &d.SessionContext{Plan: plan}, Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		return d.Outcome{Execution: "completed", Outcome: "success"}, nil
	})}
	r, err := engine.Run(context.Background(), p, "adaptive", agents(), &d.Ledger{}, nil, "down")
	if err != nil || r.Attempts != 2 {
		t.Fatal(r, err)
	}
	plan.FixedPlacement.Selected = nil
	engine.CELContext.Plan = plan
	engine.Runner = runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		t.Fatal("pending inheritance escaped Monitor placement")
		return d.Outcome{}, nil
	})
	r, err = engine.Run(context.Background(), p, "adaptive", agents(), &d.Ledger{}, nil, "down")
	if err != nil || r.Attempts != 0 {
		t.Fatal(r, err)
	}
}
