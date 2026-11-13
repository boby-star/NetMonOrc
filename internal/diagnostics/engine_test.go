package diagnostics_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
)

type runFunc func(context.Context, d.Action, d.Executor) (d.Outcome, error)

func (f runFunc) Execute(c context.Context, a d.Action, e d.Executor) (d.Outcome, error) {
	return f(c, a, e)
}
func profiles(t *testing.T) (d.Profile, d.Profile) {
	t.Helper()
	load := func(file string) d.Profile {
		b, e := os.ReadFile("../../examples/diagnostics/" + file + ".yaml")
		if e != nil {
			t.Fatal(e)
		}
		p, e := config.CompileDiagnostics(b)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	return load("adaptive"), load("fixed")
}
func agents() []d.Executor {
	return []d.Executor{{ID: "branch", Zone: "office", Online: true, Capacity: 1, Supported: map[string]bool{"http": true, "tcp": true, "dns": true, "blackbox": true}}, {ID: "dc", Zone: "server", Online: true, Capacity: 1, Supported: map[string]bool{"http": true, "tcp": true, "dns": true, "blackbox": true}}}
}
func gate(d.Action, d.Executor) bool { return true }

func TestCompareModes(t *testing.T) {
	adaptive, fixed := profiles(t)
	for _, tc := range []struct {
		name, truth string
		outcomes    map[string]string
		counts      []int
	}{
		{"dns", "dns-failure", map[string]string{"d01-dns": "failure", "d02-server-http": "success", "d03-office-tcp": "success", "d04-office-http": "success"}, []int{4, 1, 1}},
		{"service", "service-failure", map[string]string{"d01-dns": "success", "d02-server-http": "failure", "d03-office-tcp": "success", "d04-office-http": "failure"}, []int{4, 3, 2}},
		{"zone", "zone-path-failure", map[string]string{"d01-dns": "success", "d02-server-http": "success", "d03-office-tcp": "failure", "d04-office-http": "failure"}, []int{4, 3, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i, mode := range []string{"static", "fixed", "adaptive"} {
				p := adaptive
				if mode == "fixed" {
					p = fixed
				}
				engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
					return d.Outcome{Execution: "completed", Outcome: tc.outcomes[a.ID]}, nil
				})}
				r, e := engine.Run(context.Background(), p, mode, agents(), &d.Ledger{}, nil, "down")
				if r.DurationMillis == nil || *r.DurationMillis < 0 {
					t.Fatal("monotonic run duration missing")
				}
				if e != nil || r.Status != "completed" || len(r.Hypotheses) != 1 || r.Hypotheses[0] != tc.truth || r.Attempts != tc.counts[i] {
					t.Fatalf("%s: %+v %v", mode, r, e)
				}
			}
		})
	}
}
func TestFailureBudgetsAndNoRepeat(t *testing.T) {
	p, _ := profiles(t)
	calls := 0
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		calls++
		return d.Outcome{Execution: "execution_failed", Outcome: "unknown"}, nil
	})}
	ledger := &d.Ledger{}
	r, e := engine.Run(context.Background(), p, "adaptive", agents(), ledger, nil, "down")
	if e != nil || r.Status != "inconclusive" || len(r.Hypotheses) != 3 || calls != 4 {
		t.Fatalf("execution failures changed hypotheses: %+v %v", r, e)
	}
	r, e = engine.Run(context.Background(), p, "adaptive", agents(), ledger, nil, "down")
	if e != nil || r.Attempts != 0 || calls != 4 {
		t.Fatal("actions repeated without confirmation/new evidence", r, e)
	}
	p.Budget.MaxChecks = 50
	if _, e = engine.Run(context.Background(), p, "adaptive", agents(), ledger, nil, "down"); e == nil {
		t.Fatal("episode budget replaced")
	}
	p.Budget.MaxChecks = 1
	ledger = &d.Ledger{}
	r, e = engine.Run(context.Background(), p, "adaptive", agents(), ledger, nil, "down")
	if e != nil || r.Attempts != 1 || r.Reason != "check_budget_exhausted" {
		t.Fatalf("budget %+v %v", r, e)
	}
	ledger = &d.Ledger{StartedAt: time.Now().Add(-2 * time.Minute)}
	r, e = engine.Run(context.Background(), p, "adaptive", agents(), ledger, nil, "down")
	if e != nil || r.Attempts != 0 || r.Reason != "deadline_or_cancelled" {
		t.Fatal("expired episode dispatched", r, e)
	}
}
func TestNoGainAndNoAgents(t *testing.T) {
	p, _ := profiles(t)
	for i := range p.Actions {
		for h := range p.Actions[i].Expected {
			p.Actions[i].Expected[h] = "either"
		}
	}
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		t.Fatal("uninformative action executed")
		return d.Outcome{}, nil
	})}
	r, e := engine.Run(context.Background(), p, "adaptive", agents(), &d.Ledger{}, nil, "down")
	if e != nil || r.Reason != "no_gain" || r.Attempts != 0 {
		t.Fatal(r, e)
	}
	r, e = engine.Run(context.Background(), p, "adaptive", nil, &d.Ledger{}, nil, "down")
	if e != nil || r.Reason != "no_admissible_action" {
		t.Fatal(r, e)
	}
}
func TestPairGain(t *testing.T) {
	h := []string{"dns", "service", "zone"}
	if got := d.Gain(h, map[string]string{"dns": "success", "service": "either", "zone": "failure"}); got != 1 {
		t.Fatal(got)
	}
	if got := d.Gain(h, map[string]string{"dns": "failure", "service": "success", "zone": "success"}); got != 2 {
		t.Fatal(got)
	}
}
