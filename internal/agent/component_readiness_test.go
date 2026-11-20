package agent

import (
	"context"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestReadinessGroupsEndpointsAndNeverReportsUnappliedPlan(t *testing.T) {
	boot, err := bootTime()
	if err != nil {
		t.Fatal(err)
	}
	st := &state{}
	for _, name := range []string{"one", "two"} {
		st.set(&assignment{plan: monitor.Plan{Name: name, Revision: name, Components: map[string]monitor.ComponentBinding{"branch": {Ref: "exporter", Hash: "hash", Endpoint: "http://blackbox:9115", Enabled: true}}}, deadline: time.Now().Add(time.Minute), bootDeadline: boot + time.Minute})
	}
	calls := 0
	refreshComponents(context.Background(), st, "branch", func(context.Context, monitor.ComponentBinding) bool { calls++; return true })
	if calls != 1 {
		t.Fatal("one network check per Plan instead of endpoint", calls)
	}
	reports := componentReports(st, "branch", map[string]bool{"one": true})
	if len(reports) != 1 || reports[0].Revision != "one" || reports[0].State != "ready" {
		t.Fatal("unapplied session Plan leaked into heartbeat", reports)
	}
	for _, a := range st.list() {
		b := a.plan.Components["branch"]
		b.Enabled = false
		a.plan.Components["branch"] = b
	}
	refreshComponents(context.Background(), st, "branch", func(context.Context, monitor.ComponentBinding) bool {
		t.Fatal("disabled binding made a network call")
		return true
	})
	reports = componentReports(st, "branch", map[string]bool{"one": true, "two": true})
	if len(reports) != 2 || reports[0].State != "disabled" || reports[1].State != "disabled" {
		t.Fatal("disabled status missing", reports)
	}
}
