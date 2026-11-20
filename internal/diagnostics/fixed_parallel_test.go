package diagnostics_test

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func parallelProfile() d.Profile {
	action := func(id, agent string) d.Action {
		return d.Action{ID: id, Check: monitor.Check{Type: "http", Runner: "blackbox", Timeout: time.Second}, Cost: 1, Expected: map[string]string{"fault": "failure", "other": "success"}, Placement: d.Placement{Strategy: "fixed", AgentRefs: []string{agent}}}
	}
	return d.Profile{Mode: "fixed", Budget: d.Budget{MaxChecks: 3, MaxDuration: 5 * time.Second, MaxParallel: 2}, Hypotheses: []string{"fault", "other"}, Actions: []d.Action{action("left", "branch"), action("right", "dc"), action("join", "branch")}, Steps: []d.Step{{ID: "a-join", ActionID: "join", DependsOn: []string{"b-left", "c-right"}, When: func(_ context.Context, tasks map[string]d.Outcome, _ string) (bool, error) {
		return tasks["b-left"].Outcome == "success" && tasks["c-right"].Outcome == "success", nil
	}}, {ID: "b-left", ActionID: "left"}, {ID: "c-right", ActionID: "right"}}}
}

func TestFixedWaveOverlapsAndPublishesBeforeDependent(t *testing.T) {
	p := parallelProfile()
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	rightDone := make(chan struct{})
	var finished atomic.Int32
	var mu sync.Mutex
	var physical []string
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(ctx context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
		if a.ID == "join" {
			if finished.Load() != 2 {
				t.Error("dependent ran before its parents")
			}
		} else {
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return d.Outcome{}, ctx.Err()
			}
			if a.ID == "left" {
				select {
				case <-rightDone:
				case <-ctx.Done():
					return d.Outcome{}, ctx.Err()
				}
			}
		}
		mu.Lock()
		physical = append(physical, a.ID)
		mu.Unlock()
		if a.ID == "right" {
			close(rightDone)
		}
		if a.ID != "join" {
			finished.Add(1)
		}
		return d.Outcome{Execution: "completed", Outcome: "success"}, nil
	})}
	type output struct {
		r   d.Result
		err error
	}
	done := make(chan output, 1)
	go func() {
		r, err := engine.Run(context.Background(), p, "fixed", agents(), &d.Ledger{}, nil, "down")
		done <- output{r, err}
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("independent steps did not overlap")
		}
	}
	close(release)
	result := <-done
	if result.err != nil || result.r.Status != "completed" || result.r.Attempts != 3 {
		t.Fatal(result.r, result.err)
	}
	var order []string
	for _, v := range result.r.Decisions {
		order = append(order, v.StepID)
	}
	if !reflect.DeepEqual(order, []string{"b-left", "c-right", "a-join"}) || !reflect.DeepEqual(physical, []string{"right", "left", "join"}) {
		t.Fatal("completion race changed canonical trace", order, physical)
	}
	if len(result.r.Decisions[0].HypothesesBefore) != 2 || len(result.r.Decisions[1].HypothesesBefore) != 2 || len(result.r.Decisions[2].HypothesesBefore) != 1 {
		t.Fatal("wave context was mutated by a racing result")
	}
}

func TestFixedParallelRetryQuotasAndSkippedBudgetTail(t *testing.T) {
	for _, limit := range []int64{20, 30} {
		p := parallelProfile()
		p.Steps = p.Steps[1:]
		p.Budget.MaxChecks = 3
		p.Budget.MaxEstimatedBytes = limit
		for i := range p.Actions {
			p.Actions[i].Retry = &d.Retry{MaxAttempts: 2}
		}
		engine := d.Engine{Eligible: gate, EstimateBytes: func(d.Action, d.Executor) (int64, bool) { return 10, true }, Runner: runFunc(func(_ context.Context, a d.Action, e d.Executor) (d.Outcome, error) {
			if a.StepID == "b-left" && e.ID != "branch" || a.StepID == "c-right" && e.ID != "dc" {
				t.Error("retry reselected Agent")
			}
			if a.AttemptNo == 1 {
				return d.Outcome{Execution: "execution_failed", Outcome: "unknown"}, nil
			}
			return d.Outcome{Execution: "completed", Outcome: "success"}, nil
		})}
		ledger := &d.Ledger{}
		r, err := engine.Run(context.Background(), p, "fixed", agents(), ledger, nil, "down")
		if err != nil || r.Status != "inconclusive" || int64(r.Attempts) != limit/10 || ledger.EstimatedBytes != limit {
			t.Fatal("parallel retries bypassed episode quota", r, err)
		}
		if limit == 30 && (r.Decisions[2].StepID != "b-left" || r.Decisions[2].AttemptNo != 2) {
			t.Fatal("racing retry consumed the final token")
		}
	}
	p := parallelProfile()
	p.Budget.MaxChecks = 2
	p.Steps[0].When = func(context.Context, map[string]d.Outcome, string) (bool, error) { return false, nil }
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		return d.Outcome{Execution: "completed", Outcome: "success"}, nil
	})}
	r, err := engine.Run(context.Background(), p, "fixed", agents(), &d.Ledger{}, nil, "down")
	if err != nil || r.Status != "completed" || r.Attempts != 2 || !reflect.DeepEqual(r.SkippedSteps, []string{"a-join"}) {
		t.Fatal("exhausted probe budget blocked a zero-cost guard skip", r, err)
	}
}

func TestFixedParallelCancellationWaitsForWorkers(t *testing.T) {
	p := parallelProfile()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var started atomic.Int32
	var exited atomic.Int32
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(ctx context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
		if a.ID == "join" {
			t.Error("child dispatched after cancellation")
		}
		if started.Add(1) == 2 {
			cancel()
		}
		<-ctx.Done()
		exited.Add(1)
		return d.Outcome{}, ctx.Err()
	})}
	r, err := engine.Run(ctx, p, "fixed", agents(), &d.Ledger{}, nil, "down")
	if err != nil || r.Reason != "deadline_or_cancelled" || r.Attempts != 2 || exited.Load() != 2 {
		t.Fatal("planner returned before parallel workers stopped", r, err)
	}
}
