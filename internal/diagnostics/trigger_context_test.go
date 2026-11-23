package diagnostics

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
)

func TestTriggerClosedMonitorContext(t *testing.T) {
	for _, expression := range []string{`monitor.sttae == "down"`, `monitor.coverageSttae == "unknown"`, `monitor.state > 1`} {
		if _, err := CompileTrigger(expression); err == nil {
			t.Errorf("invalid trigger accepted %s", expression)
		}
	}
	trigger, err := CompileTrigger(`monitor.name == "web" && monitor.state == "down" && monitor.coverageState == "unknown"`)
	if err != nil {
		t.Fatal(err)
	}
	yes, err := trigger(context.Background(), monitor.Assessment{Monitor: "web", State: "down"})
	if err != nil || !yes {
		t.Fatal(yes, err)
	}
}

func TestTriggerLabelsAttributesAndOptionalScore(t *testing.T) {
	p := monitor.Plan{Labels: map[string]string{"environment": "research"}, TargetContext: &monitor.TargetContext{Attributes: map[string]monitor.Scalar{"serial": {Kind: "integer", Integer: 9007199254740993}}}}
	trigger, err := CompileTrigger(`monitor.labels.environment == "research" && target.attributes.serial == 9007199254740993 && !has(monitor.weightedScore)`, p)
	if err != nil {
		t.Fatal(err)
	}
	yes, err := trigger(context.Background(), monitor.Assessment{Monitor: "web", State: "down", Zones: []monitor.ZoneAssessment{{Zone: "office"}, {Zone: "server"}}})
	if err != nil || !yes {
		t.Fatal(yes, err)
	}
}
