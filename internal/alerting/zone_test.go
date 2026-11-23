package alerting

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"testing"
	"time"
)

func TestZoneLifecycleUsesOwnFactsAndGlobalContext(t *testing.T) {
	r := Rule{Name: "zone-rule", Scope: "zone", Selector: &resourcegroup.Selector{MatchLabels: map[string]string{"environment": "research"}}, When: `zone.state == "down" && checks.http.outcome == "failure" && monitor.labels.environment == "research" && target.attributes.serial == 9007199254740993`, ResolveWhen: `zone.state == "healthy"`, Enabled: true, NotifyEvents: []string{"firing", "resolved"}, Cooldown: time.Minute}
	e, err := New(r)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Add(time.Second)
	plan := monitor.Plan{Labels: map[string]string{"environment": "research"}, TargetContext: &monitor.TargetContext{Attributes: map[string]monitor.Scalar{"serial": {Kind: "integer", Integer: 9007199254740993}}}}
	office := monitor.ZoneAssessment{Zone: "office", State: "down", Coverage: "complete", Evidence: []string{"office-proof"}, Checks: map[string]monitor.CheckView{"http": {Outcome: "failure"}}}
	server := monitor.ZoneAssessment{Zone: "server", State: "healthy", Coverage: "complete", Evidence: []string{"server-proof"}, Checks: map[string]monitor.CheckView{"http": {Outcome: "success"}}}
	whole := monitor.Assessment{Monitor: "web", Revision: "r1", At: now, State: "down", Zones: []monitor.ZoneAssessment{office, server}}
	scoped := whole
	scoped.Zones = []monitor.ZoneAssessment{office}
	scoped.State = office.State
	frame := StepContext{Plan: plan, Whole: whole, Selected: true}
	firing := e.Step(context.Background(), State{}, scoped, frame)
	if firing.State.Status != "firing" || firing.Notify != "firing" {
		t.Fatal(firing)
	}
	scoped.Zones = []monitor.ZoneAssessment{server}
	scoped.State = server.State
	if got := e.Step(context.Background(), State{}, scoped, frame); got.State.Status != "normal" || got.Notify != "" {
		t.Fatal("healthy zone followed global DOWN", got)
	}
	scoped.At = now.Add(time.Second)
	scoped.Zones = []monitor.ZoneAssessment{{Zone: "office", State: "unknown"}}
	scoped.State = "unknown"
	if got := e.Step(context.Background(), firing.State, scoped, frame); got.State.Status != "firing" {
		t.Fatal("unknown resolved firing", got)
	}
	scoped.At = now.Add(2 * time.Second)
	scoped.Zones = []monitor.ZoneAssessment{{Zone: "office", State: "healthy"}}
	scoped.State = "healthy"
	frame.Selected = false
	resolved := e.Step(context.Background(), firing.State, scoped, frame)
	if resolved.State.Status != "resolved" || resolved.Notify != "resolved" {
		t.Fatal("deselected instance could not resolve", resolved)
	}
	scoped.At = now.Add(3 * time.Second)
	scoped.State = "down"
	scoped.Zones = []monitor.ZoneAssessment{office}
	if got := e.Step(context.Background(), State{}, scoped, frame); got.State.Status != "normal" {
		t.Fatal("selector mismatch started new firing", got)
	}
}
