package config

import (
	"os"
	"strings"
	"testing"
)

func TestAssessmentManifest(t *testing.T) {
	b, e := os.ReadFile("../../examples/cel.yaml")
	if e != nil {
		t.Fatal(e)
	}
	p, e := Compile(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Rules) != 2 || p.Rules[0].ID != "server-error" {
		t.Fatal("rules not normalized")
	}
	if e = VerifyPlan(p, "office"); e != nil {
		t.Fatal(e)
	}
	for _, change := range [][2]string{{"priority: 200", "priority: 200.5"}, {"priority: 200", "priority: 0"}, {"state: down", "state: invalid"}, {"checks[\"http\"].metrics.httpStatus >= 500", "monitor.state == \"healthy\""}} {
		if _, e = Compile([]byte(strings.ReplaceAll(string(b), change[0], change[1]))); e == nil {
			t.Fatalf("invalid rule config accepted: %v", change)
		}
	}
}
