package config

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

func TestCompleteFixedContextAndReplicaValues(t *testing.T) {
	expression := `monitor.name == "web" && monitor.labels.environment == "research" && monitor.weightedScore == 1.0 && target.attributes.serial == 9007199254740993 && zone.state == "healthy" && checks.http.outcome == "success" && diagnostic.attempts == 2 && diagnostic.remainingChecks == 1 && diagnostic.elapsedSeconds >= 0.0 && tasks.read.replicaResults.branch.values.depth == 0 && tasks.read.replicaResults.dc.values.depth == 10`
	condition, err := diagnosticContextCondition(expression, map[string]bool{"read": true})
	if err != nil {
		t.Fatal(err)
	}
	score := 1.0
	frame := diagnostics.FixedContext{Monitor: monitor.RuntimeMonitorView{Name: "web", Labels: map[string]string{"environment": "research"}, WeightedScore: &score}, Target: monitor.TargetView{Attributes: monitor.ScalarValues(map[string]monitor.Scalar{"serial": {Kind: "integer", Integer: 9007199254740993}})}, Zone: &monitor.EvaluatedZoneView{State: "healthy"}, Checks: map[string]monitor.CheckView{"http": {Outcome: "success"}}, Diagnostic: diagnostics.DiagnosticCounters{Attempts: 2, RemainingChecks: 1}}
	task := diagnostics.TaskEvidence{ReplicaResults: map[string]diagnostics.TaskEvidence{"branch": {Values: map[string]monitor.Scalar{"depth": {Kind: "integer", Integer: 0}}}, "dc": {Values: map[string]monitor.Scalar{"depth": {Kind: "integer", Integer: 10}}}}}
	yes, err := condition(context.Background(), map[string]diagnostics.TaskEvidence{"read": task}, frame)
	if err != nil || !yes {
		t.Fatal(yes, err)
	}
	task.ReplicaResults["dc"] = diagnostics.TaskEvidence{Values: map[string]monitor.Scalar{"depth": {Kind: "integer", Integer: 10}}, ValidUntil: time.Now().Add(-time.Second)}
	if yes, err := condition(context.Background(), map[string]diagnostics.TaskEvidence{"read": task}, frame); err == nil || yes {
		t.Fatal("expired replica value accepted", yes, err)
	}
}
func TestFixedContextReferencesValidatedOnMonitorBinding(t *testing.T) {
	p := monitor.Plan{Checks: []monitor.Check{{ID: "http"}}, Zones: []string{"office"}}
	for _, expression := range []string{`checks.typo.outcome == "failure"`, `zones.office.checks.typo.outcome == "failure"`, `zones.missing.state == "down"`} {
		profile := diagnostics.Profile{Steps: []diagnostics.Step{{ID: "guard", Expression: expression}}}
		if err := ValidateDiagnosticMonitor(p, profile); err == nil {
			t.Errorf("unresolved context reference accepted: %s", expression)
		}
	}
}
