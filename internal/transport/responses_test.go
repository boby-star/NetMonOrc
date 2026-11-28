package transport

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

func TestResponseRequiresCurrentCompleteEvidence(t *testing.T) {
	p := monitor.Plan{Name: "web", Revision: "current"}
	a := monitor.Assessment{Monitor: "web", Revision: "current", State: "down", At: time.Now(), Zones: []monitor.ZoneAssessment{{Zone: "office", Coverage: "complete", Evidence: []string{"proof"}}}}
	if !ResponseEligible(p, a) {
		t.Fatal("current complete negative evidence rejected")
	}
	for _, mutate := range []func(*monitor.Assessment){func(a *monitor.Assessment) { a.State = "unknown" }, func(a *monitor.Assessment) { a.Revision = "old" }, func(a *monitor.Assessment) { a.At = time.Now().Add(-10 * time.Second) }, func(a *monitor.Assessment) { a.Zones = nil }, func(a *monitor.Assessment) {
		a.Zones = []monitor.ZoneAssessment{{Coverage: "coverage_degraded", Evidence: []string{"proof"}}}
	}, func(a *monitor.Assessment) { a.Zones = []monitor.ZoneAssessment{{Coverage: "complete"}} }} {
		invalid := a
		mutate(&invalid)
		if ResponseEligible(p, invalid) {
			t.Fatal("unsafe response admission")
		}
	}
}
