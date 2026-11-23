package config

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
)

func TestFixedEvidencePreservesTypedValuesAndMissing(t *testing.T) {
	condition, err := diagnosticEvidenceCondition(`tasks["previous"].values.serial == 9007199254740993 && tasks["previous"].values.depth == 0 && !tasks["previous"].values.ready && tasks["previous"].values.ratio == 1.0 && tasks["previous"].reasonCode == "HTTP_UNEXPECTED_STATUS"`, map[string]bool{"previous": true})
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]diagnostics.TaskEvidence{"previous": {Outcome: diagnostics.Outcome{Execution: "completed", Outcome: "failure"}, ReasonCode: "HTTP_UNEXPECTED_STATUS", Values: map[string]monitor.Scalar{"serial": {Kind: "integer", Integer: 9007199254740993}, "depth": {Kind: "integer", Integer: 0}, "ready": {Kind: "boolean", Boolean: false}, "ratio": {Kind: "float", Number: 1}}}}
	yes, err := condition(context.Background(), data, "down")
	if err != nil || !yes {
		t.Fatal(yes, err)
	}
	proof := data["previous"]
	proof.Values = nil
	data["previous"] = proof
	if yes, err := condition(context.Background(), data, "down"); err == nil || yes {
		t.Fatal("missing extraction fabricated", yes, err)
	}
}
