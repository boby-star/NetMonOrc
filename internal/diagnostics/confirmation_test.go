package diagnostics_test

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/config"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"os"
	"testing"
	"time"
)

func confirmedLab(t *testing.T) d.Profile {
	t.Helper()
	b, e := os.ReadFile("../../deployment/lab/adaptive.yaml")
	if e != nil {
		t.Fatal(e)
	}
	p, e := config.CompileDiagnostics(b)
	if e != nil {
		t.Fatal(e)
	}
	p.ContinueAfterUnique = true
	p.Confirmation = &d.Confirmation{ActionRefs: []string{"d01-office-dns", "d02-server-dns", "d03-server-tcp", "d04-server-http"}, MinObservations: 4, MinDistinctFailureDomains: 2}
	return p
}
func independentAgents() []d.Executor {
	a := agents()
	for i := range a {
		a[i].FailureDomain = a[i].Zone
	}
	return a
}
func TestConfirmationCrossLayerFaults(t *testing.T) {
	for _, truth := range []string{"http-failure", "dns-failure", "tcp-failure", "office-path-failure", "multiple", "outside-catalog"} {
		t.Run(truth, func(t *testing.T) {
			p := confirmedLab(t)
			engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
				result := a.Expected[truth]
				if truth == "multiple" {
					result = "success"
					if a.Check.Type == "dns" || a.Check.Type == "http" {
						result = "failure"
					}
				}
				if truth == "outside-catalog" {
					result = "success"
				}
				return d.Outcome{Execution: "completed", Outcome: result}, nil
			})}
			r, e := engine.Run(context.Background(), p, "adaptive", independentAgents(), &d.Ledger{}, nil, "down")
			if e != nil {
				t.Fatal(e)
			}
			if truth == "multiple" || truth == "outside-catalog" {
				if r.Status != "inconclusive" || r.Reason != "evidence_conflict" {
					t.Fatalf("unmodeled fault became a class: %+v", r)
				}
			} else if r.Status != "completed" || r.Reason != "confirmed_class" || r.Hypotheses[0] != truth || r.Attempts != 4 || !r.Confirmation.Satisfied {
				t.Fatalf("confirmation failed: %+v", r)
			}
			phase := false
			for _, decision := range r.Decisions {
				phase = phase || decision.Phase == "confirmation"
				if decision.Phase == "confirmation" && decision.Score != 0 {
					t.Fatal("confirmation changed Gain/Cost")
				}
			}
			if !phase {
				t.Fatal("early-stop bypassed required confirmation")
			}
		})
	}
}
func repeatProfile() d.Profile {
	return d.Profile{Mode: "adaptive", Hypotheses: []string{"fault", "other"}, Budget: d.Budget{MaxChecks: 3, MaxDuration: time.Second, MaxParallel: 1},
		Actions:      []d.Action{{ID: "verify", Check: monitor.Check{Type: "http", Runner: "blackbox", Timeout: 100 * time.Millisecond}, Cost: 1, Expected: map[string]string{"fault": "failure", "other": "success"}, Placement: d.Placement{Strategy: "leastLoaded"}}},
		Confirmation: &d.Confirmation{ActionRefs: []string{"verify"}, MinObservations: 2, MinDistinctFailureDomains: 2}, ContinueAfterUnique: true}
}
func failureRunner(_ context.Context, _ d.Action, _ d.Executor) (d.Outcome, error) {
	return d.Outcome{Execution: "completed", Outcome: "failure"}, nil
}
func TestConfirmationIndependentDomains(t *testing.T) {
	for _, shared := range []bool{false, true} {
		a := independentAgents()
		if shared {
			a[1].FailureDomain = a[0].FailureDomain
		}
		p := repeatProfile()
		engine := d.Engine{Eligible: gate, Runner: runFunc(failureRunner)}
		r, e := engine.Run(context.Background(), p, "adaptive", a, &d.Ledger{}, nil, "down")
		if e != nil {
			t.Fatal(e)
		}
		if shared {
			if r.Status != "inconclusive" || r.Attempts != 1 || r.Confirmation.DistinctFailureDomains != 1 {
				t.Fatal("shared path counted independent", r)
			}
		} else if r.Status != "completed" || r.Attempts != 2 || r.Decisions[0].AgentID == r.Decisions[1].AgentID {
			t.Fatal("independent confirmation failed", r)
		}
	}
}
func TestConfirmationBudgetAndUnusableEvidence(t *testing.T) {
	for _, tc := range []struct {
		name      string
		limit     int
		failGuard bool
	}{{"budget", 2, false}, {"technical", 6, true}, {"unknown", 6, true}} {
		t.Run(tc.name, func(t *testing.T) {
			p := confirmedLab(t)
			p.Budget.MaxChecks = tc.limit
			engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
				if tc.failGuard && a.ID == "d04-server-http" {
					if tc.name == "unknown" {
						return d.Outcome{Execution: "completed", Outcome: "unknown"}, nil
					}
					return d.Outcome{Execution: "execution_failed", Outcome: "unknown"}, nil
				}
				return d.Outcome{Execution: "completed", Outcome: a.Expected["http-failure"]}, nil
			})}
			r, e := engine.Run(context.Background(), p, "adaptive", independentAgents(), &d.Ledger{}, nil, "down")
			if e != nil || r.Status != "inconclusive" || r.Confirmation.Satisfied || r.Attempts > tc.limit {
				t.Fatal("unsafe confirmation completion", r, e)
			}
			if tc.failGuard && r.Attempts != 4 {
				t.Fatal("technical failure retried through confirmation", r)
			}
		})
	}
}
func TestConfirmationSpacingAndCancellation(t *testing.T) {
	p := repeatProfile()
	p.Confirmation.MinDistinctFailureDomains = 1
	p.Confirmation.MinSpacing = 20 * time.Millisecond
	var starts []time.Time
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(c context.Context, a d.Action, e d.Executor) (d.Outcome, error) {
		starts = append(starts, time.Now())
		return failureRunner(c, a, e)
	})}
	r, e := engine.Run(context.Background(), p, "adaptive", independentAgents(), &d.Ledger{}, nil, "down")
	if e != nil || r.Status != "completed" || len(starts) != 2 || starts[1].Sub(starts[0]) < p.Confirmation.MinSpacing {
		t.Fatal("spacing not enforced before dispatch", r, e)
	}
	p.Confirmation.MinSpacing = 200 * time.Millisecond
	starts = nil
	ctx, done := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer done()
	r, e = engine.Run(ctx, p, "adaptive", independentAgents(), &d.Ledger{}, nil, "down")
	if e != nil || r.Status != "inconclusive" || len(starts) != 1 {
		t.Fatal("probe dispatched during cancelled confirmation wait", r, e)
	}
}
func TestBaselineAliasDoesNotCreateIndependentObservation(t *testing.T) {
	p := repeatProfile()
	p.Confirmation.MinDistinctFailureDomains = 1
	alias := p.Actions[0]
	alias.ID = "alias"
	p.Actions = append(p.Actions, alias)
	p.Confirmation.ActionRefs = append(p.Confirmation.ActionRefs, "alias")
	proof := d.Evidence{ActionID: "verify", AgentID: "branch", ObservationID: "one-physical-observation", ValidUntil: time.Now().Add(time.Second), Outcome: d.Outcome{Execution: "completed", Outcome: "failure"}}
	other := proof
	other.ActionID = "alias"
	engine := d.Engine{Eligible: gate, Runner: runFunc(failureRunner)}
	r, e := engine.Run(context.Background(), p, "adaptive", independentAgents(), &d.Ledger{}, []d.Evidence{proof, other}, "down")
	if e != nil || r.Status != "completed" || r.Attempts != 1 || r.Confirmation.Observations != 2 {
		t.Fatal("alias inflated evidence count", r, e)
	}
}
func TestFixedCannotConfirmThroughTechnicalFailure(t *testing.T) {
	p := confirmedLab(t)
	p.Mode = "fixed"
	p.Confirmation = nil
	p.ContinueAfterUnique = false
	for i, id := range []string{"d01-office-dns", "d02-server-dns", "d03-server-tcp", "d04-server-http"} {
		p.Steps = append(p.Steps, d.Step{ID: string(rune('a' + i)), ActionID: id})
	}
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, a d.Action, _ d.Executor) (d.Outcome, error) {
		if a.Check.Type == "http" {
			return d.Outcome{Execution: "execution_failed", Outcome: "unknown"}, nil
		}
		return d.Outcome{Execution: "completed", Outcome: a.Expected["dns-failure"]}, nil
	})}
	r, e := engine.Run(context.Background(), p, "fixed", independentAgents(), &d.Ledger{}, nil, "down")
	if e != nil || r.Status != "inconclusive" || r.Reason != "fixed_incomplete_evidence" {
		t.Fatal("Fixed ignored unavailable confirmation step", r, e)
	}
}

func TestExpiredInitialEvidenceCannotDetermineFinalClass(t *testing.T) {
	p := repeatProfile()
	p.Confirmation.MinDistinctFailureDomains = 1
	p.Confirmation.MinSpacing = 20 * time.Millisecond
	proof := d.Evidence{ActionID: "verify", AgentID: "branch", ObservationID: "expires", ValidUntil: time.Now().Add(5 * time.Millisecond), Outcome: d.Outcome{Execution: "completed", Outcome: "failure"}}
	engine := d.Engine{Eligible: gate, Runner: runFunc(failureRunner)}
	r, e := engine.Run(context.Background(), p, "adaptive", independentAgents(), &d.Ledger{}, []d.Evidence{proof}, "down")
	if e != nil || r.Status != "inconclusive" || r.Reason != "initial_evidence_expired" {
		t.Fatal("stale baseline supported a final class", r, e)
	}
}
func TestConcurrentEpisodeRunIsFenced(t *testing.T) {
	p := repeatProfile()
	p.Confirmation = nil
	p.ContinueAfterUnique = false
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(c context.Context, a d.Action, e d.Executor) (d.Outcome, error) {
		close(entered)
		<-release
		return failureRunner(c, a, e)
	})}
	ledger := &d.Ledger{}
	go func() {
		_, e := engine.Run(context.Background(), p, "adaptive", independentAgents(), ledger, nil, "down")
		done <- e
	}()
	<-entered
	if _, e := engine.Run(context.Background(), p, "adaptive", independentAgents(), ledger, nil, "down"); e == nil {
		t.Fatal("concurrent run consumed the same episode")
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
