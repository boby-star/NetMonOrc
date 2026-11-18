package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticExplanationProjection(t *testing.T) {
	raw := []byte(`{"hypotheses":["dns"],"durationMs":0,"evidencePolicy":{"maxAgeMs":20000,"maxSkewMs":15000},"costUnits":2,"decisions":[{"actionId":"dns-check","agentId":"branch","observationId":"proof","finishedAt":"2026-11-18T12:00:00Z","validUntil":"2026-11-18T12:00:20Z","failureDomain":"shared-gateway","attemptNo":2,"phase":"confirmation","hypothesesBefore":["dns","http"],"gain":1,"score":0.5,"result":{"Execution":"execution_failed","Outcome":"unknown","secret":"must-not-expose"},"target":"must-not-expose"}],"confirmation":{"requiredActions":["dns-check"],"observedActions":[],"observations":0,"distinctFailureDomains":0,"satisfied":false,"countedEvidence":["must-not-expose"],"evidenceDomains":{"attempt:1":"shared-gateway"}},"blockedSteps":["tcp"],"skippedSteps":["http"],"private":"must-not-expose"}`)
	v, err := diagnosticExplanation(raw)
	if err != nil || v == nil || v.DurationMS == nil || *v.DurationMS != 0 || v.CostUnits != 2 || len(v.Decisions) != 1 {
		t.Fatalf("report lost: %+v %v", v, err)
	}
	d := v.Decisions[0]
	if d.ObservationID != "proof" || d.FinishedAt == nil || d.ValidUntil == nil || v.EvidencePolicy == nil || v.EvidencePolicy.MaxSkewMS != 15000 || d.FailureDomain != "shared-gateway" || d.ExecutionStatus != "execution_failed" || d.ProbeOutcome != "unknown" || d.AttemptNo != 2 || d.Gain != 1 || d.Score != 0.5 || len(d.HypothesesBefore) != 2 {
		t.Fatalf("decision lost: %+v", d)
	}
	if v.Confirmation == nil || v.Confirmation.EvidenceDomains["attempt:1"] != "shared-gateway" || v.Confirmation.Satisfied || v.Confirmation.Observations != 0 || len(v.BlockedSteps) != 1 || len(v.SkippedSteps) != 1 {
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

func TestBaselineEvidenceProjectionAndLegacy(t *testing.T) {
	raw := []byte(`{"decisions":[],"baselineEvidence":[{"observationId":"regular","actionId":"dns-action","checkId":"dns","agentId":"branch","zone":"office","configRevision":"old","finishedAt":"2026-11-18T12:00:00Z","validUntil":"2026-11-18T12:00:20Z","executionStatus":"completed","probeOutcome":"failure","target":"secret"}]}`)
	v, err := diagnosticExplanation(raw)
	if err != nil || v == nil || v.BaselineEvidence == nil || len(*v.BaselineEvidence) != 1 {
		t.Fatal(v, err)
	}
	b := (*v.BaselineEvidence)[0]
	if b.ObservationID != "regular" || b.ConfigRevision != "old" || b.CheckID != "dns" || b.ProbeOutcome != "failure" {
		t.Fatal(b)
	}
	encoded, _ := json.Marshal(v)
	if strings.Contains(string(encoded), "secret") {
		t.Fatal("private baseline field exposed")
	}
	for _, input := range []string{`{"decisions":[]}`, `{"decisions":[],"baselineEvidence":[]}`} {
		v, err := diagnosticExplanation([]byte(input))
		if err != nil || v == nil || (v.BaselineEvidence != nil) != strings.Contains(input, "baselineEvidence") {
			t.Fatal("unknown and known-empty baseline conflated", v, err)
		}
	}
}
