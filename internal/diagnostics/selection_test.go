package diagnostics_test

import (
	"context"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"testing"
	"time"
)

func TestPlacementAndRepeatedFixedAttempts(t *testing.T) {
	p, _ := profiles(t)
	p.Actions = p.Actions[1:2]
	p.Hypotheses = []string{"service-failure", "zone-path-failure"}
	delete(p.Actions[0].Expected, "dns-failure")
	a := p.Actions[0]
	a.Placement = d.Placement{Strategy: "leastLoaded"}
	p.Actions = []d.Action{a}
	executors := agents()
	executors[0].Zone = "server"
	executors[0].Capacity = 2
	executors[0].Active = 1
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		return d.Outcome{Execution: "completed", Outcome: "success"}, nil
	})}
	r, e := engine.Run(context.Background(), p, "adaptive", executors, &d.Ledger{}, nil, "down")
	if e != nil || r.Decisions[0].AgentID != "dc" {
		t.Fatal("leastLoaded ignored", r, e)
	}
	p.Mode = "fixed"
	p.Actions[0].Placement.Strategy = "roundRobin"
	p.Budget.MaxChecks = 2
	p.Steps = []d.Step{{ID: "first", ActionID: a.ID}, {ID: "second", ActionID: a.ID, DependsOn: []string{"first"}}}
	r, e = engine.Run(context.Background(), p, "fixed", executors, &d.Ledger{}, nil, "down")
	if e != nil || r.Status != "completed" || r.Attempts != 2 || r.Decisions[0].AgentID == r.Decisions[1].AgentID {
		t.Fatal("fixed repeats must consume two physical attempts and honor roundRobin", r, e)
	}
}
func TestNoConclusionWithoutSpecificEvidence(t *testing.T) {
	p, _ := profiles(t)
	p.Actions = p.Actions[:1]
	p.Hypotheses = []string{"dns-failure", "service-failure"}
	p.Actions[0].Expected = map[string]string{"dns-failure": "unknown", "service-failure": "success"}
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		return d.Outcome{Execution: "completed", Outcome: "unknown"}, nil
	})}
	r, e := engine.Run(context.Background(), p, "adaptive", agents(), &d.Ledger{}, nil, "down")
	if e != nil || r.Status != "inconclusive" || r.Reason != "insufficient_support" {
		t.Fatal("unknown-only evidence produced a diagnosis", r, e)
	}
}
func TestBudgetEstimatesAndDeadline(t *testing.T) {
	p, _ := profiles(t)
	p.Budget.MaxEstimatedBytes = 100
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		t.Fatal("dispatched without known byte estimate")
		return d.Outcome{}, nil
	})}
	r, e := engine.Run(context.Background(), p, "adaptive", agents(), &d.Ledger{}, nil, "down")
	if e != nil || r.Attempts != 0 || r.Reason != "no_admissible_action" {
		t.Fatal(r, e)
	}
	ledger := &d.Ledger{StartedAt: time.Now().Add(time.Hour)}
	if _, e = engine.Run(context.Background(), p, "adaptive", agents(), ledger, nil, "down"); e == nil {
		t.Fatal("future episode timestamp extended budget")
	}
}
