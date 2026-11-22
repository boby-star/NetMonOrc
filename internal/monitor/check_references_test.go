package monitor

import (
	"strings"
	"testing"
)

func TestAssessmentStaticCheckReferences(t *testing.T) {
	for _, expression := range []string{
		`checks["typo"].outcome == "failure"`,
		`checks.typo.outcome == "failure"`,
		`has(checks.typo)`,
		`"typo" in checks`,
		`false && checks["typo"].outcome == "success"`,
		`[1].exists(x, checks["typo"].outcome == "success")`,
		`{"value": checks["typo"].outcome}["value"] == "success"`,
	} {
		p := Plan{Checks: []Check{{ID: "http"}}, Rules: []AssessmentRule{{ID: "static-ref", Priority: 100, When: expression, State: "down", Reason: "PROBE_FAILED"}}}
		if _, err := NewAssessor(p); err == nil || !strings.Contains(err.Error(), `unknown check "typo"`) {
			t.Errorf("unresolved reference %s: %v", expression, err)
		}
	}
	for _, expression := range []string{
		`checks["http"].outcome == "failure"`, `has(checks.http)`, `"http" in checks`,
		`[{"typo": true}].exists(checks, checks.typo)`,
		`["http"].exists(id, checks[id].outcome == "success")`,
	} {
		p := Plan{Checks: []Check{{ID: "http"}}, Rules: []AssessmentRule{{ID: "valid-ref", Priority: 100, When: expression, State: "down", Reason: "PROBE_FAILED"}}}
		if _, err := NewAssessor(p); err != nil {
			t.Errorf("valid/shadowed reference %s: %v", expression, err)
		}
	}
}
