package config

import (
	"os"
	"strings"
	"testing"
)

func TestFixedConditionsValidateFieldsAndPredecessors(t *testing.T) {
	source, err := os.ReadFile("../../examples/diagnostics/fixed.yaml")
	if err != nil {
		t.Fatal(err)
	}
	original := `tasks["s01-dns"].outcome == "success"`
	for _, expression := range []string{
		`tasks["missing"].outcome == "success"`,
		`tasks["s02-tcp"].outcome == "success"`,
		`tasks["s03-http"].outcome == "success"`,
		`"missing" in tasks`,
		`tasks["s01-dns"].outcomme == "success"`,
		`monitor.sttae == "down"`,
	} {
		input := strings.Replace(string(source), original, expression, 1)
		if _, err := CompileDiagnostics([]byte(input)); err == nil {
			t.Errorf("invalid fixed condition admitted: %s", expression)
		}
	}
	if _, err := CompileDiagnostics([]byte(strings.Replace(string(source), "dependsOn: [s01-dns]", "dependsOn: []", 1))); err == nil {
		t.Fatal("undeclared predecessor admitted")
	}
	for _, expression := range []string{
		original, `tasks["s01-dns"].values.depth > 0`, `tasks["s01-dns"].reasonCode == "OK"`, `tasks["s01-dns"].executionStatus == "completed"`,
		`["s01-dns"].exists(id, tasks[id].outcome == "success")`,
		`[{"other": true}].exists(tasks, tasks.other)`,
	} {
		if _, err := CompileDiagnostics([]byte(strings.Replace(string(source), original, expression, 1))); err != nil {
			t.Errorf("valid condition rejected %s: %v", expression, err)
		}
	}
	// The existing s03-http condition reads transitive predecessor s01-dns.
	if _, err := CompileDiagnostics(source); err != nil {
		t.Fatal("transitive predecessor rejected", err)
	}
}
