package config

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
)

func TestDiagnosticProfiles(t *testing.T) {
	b, e := os.ReadFile("../../examples/diagnostics/adaptive.yaml")
	if e != nil {
		t.Fatal(e)
	}
	for _, change := range [][2]string{{"costUnits: 1", "costUnits: 0"}, {"costUnits: 1", "costUnits: .nan"}, {"maxAdditionalChecks: 6", "maxAdditionalChecks: 0"}, {"adaptive: {}", "adaptive: {stopWhenUnique: false}"}, {"adaptive: {}", "adaptive: {maxNoGainSteps: 1}"}, {"strategy: fixed", "strategy: fixed, replicas: 2"}, {"mode: adaptive", "mode: static"}, {"dns-failure: failure, service-failure: success, zone-path-failure: success", "dns-failure: failure"}} {
		if _, e = CompileDiagnostics([]byte(strings.ReplaceAll(string(b), change[0], change[1]))); e == nil {
			t.Fatalf("invalid profile accepted: %v", change)
		}
	}
	b, e = os.ReadFile("../../examples/diagnostics/fixed.yaml")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = CompileDiagnostics([]byte(strings.ReplaceAll(string(b), "dependsOn: [s01-dns]", "dependsOn: [s03-http]"))); e == nil {
		t.Fatal("cyclic DAG accepted")
	}
	condition, e := diagnosticCondition(`tasks["previous"].outcome != "success"`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = condition(context.Background(), map[string]diagnostics.Outcome{}, "down"); e == nil {
		t.Fatal("missing CEL evidence treated as ordinary false/true")
	}
	if _, e = diagnosticCondition(`monitor.state`); e == nil {
		t.Fatal("nonboolean CEL accepted")
	}
}
