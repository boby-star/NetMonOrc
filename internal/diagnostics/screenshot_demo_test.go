package diagnostics_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/config"
	d "github.com/boby-star/NetMonOrc/internal/diagnostics"
	"gopkg.in/yaml.v3"
)

// Exercise the shipped manifest, including gain/cost, placement and early stop.
func TestScreenshotAdaptiveManifest(t *testing.T) {
	data, err := os.ReadFile("../../examples/diagnostics/screenshot-demo/manifest.yaml")
	if err != nil {
		t.Fatal(err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var profile d.Profile
	for {
		var document yaml.Node
		if err := decoder.Decode(&document); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		var envelope struct{ Kind string }
		if err := document.Decode(&envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Kind == "DiagnosticProfile" {
			encoded, err := yaml.Marshal(&document)
			if err != nil {
				t.Fatal(err)
			}
			profile, err = config.CompileDiagnostics(encoded)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, scenario := range []struct {
		name     string
		outcomes []string // TCP office, dependency office, readiness server.
		steps    []string
	}{
		{"application-error", []string{"success", "success", "failure"}, []string{"a-office-tcp", "b-office-backend"}},
		{"backend-error", []string{"success", "failure", "failure"}, []string{"a-office-tcp", "b-office-backend", "c-server-ready"}},
		{"office-access-restriction", []string{"success", "failure", "success"}, []string{"a-office-tcp", "b-office-backend", "c-server-ready"}},
		{"listener-unavailable", []string{"failure", "failure", "failure"}, []string{"a-office-tcp"}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			outcomes := map[string]string{"a-office-tcp": scenario.outcomes[0], "b-office-backend": scenario.outcomes[1], "c-server-ready": scenario.outcomes[2]}
			engine := d.Engine{Eligible: gate, Runner: runFunc(func(_ context.Context, action d.Action, executor d.Executor) (d.Outcome, error) {
				want := "branch"
				if action.ID == "c-server-ready" {
					want = "dc"
				}
				if executor.ID != want {
					t.Fatalf("%s dispatched to %s instead of %s", action.ID, executor.ID, want)
				}
				return d.Outcome{Execution: "completed", Outcome: outcomes[action.ID]}, nil
			})}
			executors := agents()
			for i := range executors {
				executors[i].Supported["native-http"] = true
			}
			result, err := engine.Run(context.Background(), profile, "adaptive", executors, &d.Ledger{}, nil, "down")
			var steps []string
			for _, decision := range result.Decisions {
				steps = append(steps, decision.ActionID)
			}
			if err != nil || result.Status != "completed" || result.Reason != "supported_class" || !reflect.DeepEqual(result.Hypotheses, []string{scenario.name}) || !reflect.DeepEqual(steps, scenario.steps) {
				t.Fatalf("result=%+v steps=%v error=%v", result, steps, err)
			}
			if result.Decisions[0].Gain != 3 || result.Decisions[0].Score != 6 {
				t.Fatalf("unexpected first ranking: %+v", result.Decisions[0])
			}
			t.Logf("%s: steps=%v attempts=%d cost=%g", scenario.name, steps, result.Attempts, result.Cost)
		})
	}
}
