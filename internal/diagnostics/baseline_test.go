package diagnostics_test

import (
	"context"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

func TestBaselineEquivalenceAndFreshness(t *testing.T) {
	p, _ := profiles(t)
	c := p.Actions[0].Check
	c.ID = "regular-dns"
	c.Interval = 30 * time.Second
	c.MaxAge = time.Minute
	c.Weight = 1
	c.Required = true
	now := time.Now().UTC()
	plan := monitor.Plan{Name: "web", Revision: "current", MaxAge: time.Minute, MaxSkew: 10 * time.Second, Checks: []monitor.Check{c}}
	o := monitor.Observation{ID: "first", Monitor: "web", Revision: "current", Check: c.ID, Agent: "branch", Zone: "office", StartedAt: now.Add(-time.Second), FinishedAt: now, Execution: "completed", Outcome: "failure"}
	if got := d.MatchBaseline(plan, p, []monitor.Observation{o}, agents(), now); len(got) != 1 || got[0].ActionID != "d01-dns" {
		t.Fatalf("matching baseline missing: %+v", got)
	}
	sources := agents()
	for i := range sources {
		sources[i].FailureDomain = "new-domain"
		sources[i].EvidenceNotBefore = now.Add(-500 * time.Millisecond)
	}
	if got := d.MatchBaseline(plan, p, []monitor.Observation{o}, sources, now); len(got) != 0 {
		t.Fatal("old probe reinterpreted under new failure-domain mapping")
	}
	fresh := o
	fresh.StartedAt = now.Add(-100 * time.Millisecond)
	if got := d.MatchBaseline(plan, p, []monitor.Observation{fresh}, sources, now); len(got) != 1 {
		t.Fatal("fresh probe after trusted topology not admitted")
	}
	old := o
	old.Revision = "previous"
	if got := d.MatchBaseline(plan, p, []monitor.Observation{old}, agents(), now); len(got) != 0 {
		t.Fatal("old revision used")
	}
	o.FinishedAt = now.Add(-time.Second)
	newer := o
	newer.ID = "newer"
	newer.FinishedAt = now
	newer.Execution = "execution_failed"
	newer.Outcome = "unknown"
	if got := d.MatchBaseline(plan, p, []monitor.Observation{o, newer}, agents(), now); len(got) != 0 {
		t.Fatal("older success/failure resurrected after technical error")
	}
	plan.Checks[0].Timeout++
	if got := d.MatchBaseline(plan, p, []monitor.Observation{o}, agents(), now); len(got) != 0 {
		t.Fatal("different timeout treated as equivalent")
	}
}

func TestBaselineOnlyConclusionPreservesPhysicalInputs(t *testing.T) {
	p, _ := profiles(t)
	p.Actions = p.Actions[:1]
	p.Hypotheses = []string{"dns-failure", "service-failure"}
	p.Actions[0].Expected = map[string]string{"dns-failure": "failure", "service-failure": "success"}
	c := p.Actions[0].Check
	c.ID = "regular-dns"
	now := time.Now().UTC()
	plan := monitor.Plan{Name: "web", Revision: "historical-revision", MaxAge: time.Minute, MaxSkew: 10 * time.Second, Checks: []monitor.Check{c}}
	o := monitor.Observation{ID: "regular-proof", Monitor: plan.Name, Revision: plan.Revision, Check: c.ID, Agent: "branch", Zone: "office", StartedAt: now.Add(-time.Second), FinishedAt: now, Execution: "completed", Outcome: "failure"}
	sources := agents()
	sources[0].FailureDomain = "branch-uplink"
	initial := d.MatchBaseline(plan, p, []monitor.Observation{o}, sources, now)
	engine := d.Engine{Eligible: gate, Runner: runFunc(func(context.Context, d.Action, d.Executor) (d.Outcome, error) {
		t.Fatal("baseline conclusion launched a new probe")
		return d.Outcome{}, nil
	})}
	r, err := engine.Run(context.Background(), p, "adaptive", sources, &d.Ledger{}, initial, "down")
	if err != nil || r.Status != "completed" || r.Attempts != 0 || r.BaselineEvidence == nil || len(*r.BaselineEvidence) != 1 {
		t.Fatalf("missing zero-attempt provenance: %+v %v", r, err)
	}
	b := (*r.BaselineEvidence)[0]
	if b.ObservationID != o.ID || b.CheckID != c.ID || b.ConfigRevision != plan.Revision || b.ActionID != p.Actions[0].ID || b.Zone != o.Zone || b.FailureDomain != "branch-uplink" || !b.FinishedAt.Equal(o.FinishedAt) || !b.ValidUntil.Equal(o.FinishedAt.Add(time.Minute)) {
		t.Fatalf("physical provenance lost: %+v", b)
	}
}
