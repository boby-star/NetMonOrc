package diagnostics

import (
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"testing"
)

func TestDiagnosticPlacementIntersectsTrustedFilters(t *testing.T) {
	p := Placement{AgentRefs: []string{"branch"}, Zones: []string{"office"}, Selector: &resourcegroup.Selector{MatchLabels: map[string]string{"site": "office"}}, DistinctBy: "failureDomain"}
	good := Executor{ID: "branch", Zone: "office", FailureDomain: "office-path", Labels: map[string]string{"site": "office"}}
	if !placementMatch(p, good) {
		t.Fatal("matching trusted executor rejected")
	}
	for _, bad := range []Executor{{ID: "dc", Zone: "office", FailureDomain: "x", Labels: good.Labels}, {ID: "branch", Zone: "server", FailureDomain: "x", Labels: good.Labels}, {ID: "branch", Zone: "office", FailureDomain: "x"}, {ID: "branch", Zone: "office", Labels: good.Labels}} {
		if placementMatch(p, bad) {
			t.Fatalf("filter bypassed: %+v", bad)
		}
	}
}
