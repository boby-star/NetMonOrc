package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticExplanationProjection(t *testing.T) {
	raw := []byte(`{"hypotheses":["dns"],"durationMs":0,"costUnits":2,"decisions":[{"actionId":"dns-check","agentId":"branch","attemptNo":2,"phase":"confirmation","hypothesesBefore":["dns","http"],"gain":1,"score":0.5,"result":{"Execution":"execution_failed","Outcome":"unknown","secret":"must-not-expose"},"target":"must-not-expose"}],"confirmation":{"requiredActions":["dns-check"],"observedActions":[],"observations":0,"distinctFailureDomains":0,"satisfied":false,"countedEvidence":["must-not-expose"]},"blockedSteps":["tcp"],"skippedSteps":["http"],"private":"must-not-expose"}`)
	v, err := diagnosticExplanation(raw)
	if err != nil || v == nil || v.DurationMS == nil || *v.DurationMS != 0 || v.CostUnits != 2 || len(v.Decisions) != 1 {
		t.Fatalf("report lost: %+v %v", v, err)
	}
	d := v.Decisions[0]
	if d.ExecutionStatus != "execution_failed" || d.ProbeOutcome != "unknown" || d.AttemptNo != 2 || d.Gain != 1 || d.Score != 0.5 || len(d.HypothesesBefore) != 2 {
		t.Fatalf("decision lost: %+v", d)
	}
	if v.Confirmation == nil || v.Confirmation.Satisfied || v.Confirmation.Observations != 0 || len(v.BlockedSteps) != 1 || len(v.SkippedSteps) != 1 {
		t.Fatal("confirmation/steps lost")
	}
	encoded, err := json.Marshal(v)
	if err != nil || strings.Contains(string(encoded), "must-not-expose") {
		t.Fatal("private report fields exposed")
	}
	for _, legacy := range []string{"", "null", `{"hypotheses":["dns"]}`} {
		got, err := diagnosticExplanation([]byte(legacy))
		if err != nil || got != nil {
			t.Fatal("invented legacy journal", got, err)
		}
	}
	empty, err := diagnosticExplanation([]byte(`{"decisions":[],"costUnits":0}`))
	if err != nil || empty == nil || empty.Decisions == nil {
		t.Fatal("known empty journal lost")
	}
	if _, err = diagnosticExplanation([]byte(`{"decisions":`)); err == nil {
		t.Fatal("corrupt report accepted")
	}
}
