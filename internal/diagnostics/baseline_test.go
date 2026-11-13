package diagnostics_test

import (
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
