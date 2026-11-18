package config

import (
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"os"
	"strings"
	"testing"
	"time"
)

func TestYAMLControlsAndDefaultRevision(t *testing.T) {
	b, _ := os.ReadFile("../../examples/http.yaml")
	base, err := Compile(b)
	if err != nil {
		t.Fatal(err)
	}
	explicit := strings.Replace(string(b), "spec:\n", "spec:\n  enabled: true\n", 1)
	explicit = strings.Replace(explicit, "    - id: http\n", "    - id: http\n      enabled: true\n      priority: 50\n      jitter: 0s\n", 1)
	explicit = strings.Replace(explicit, "    address: 127.0.0.1", "    address: 127.0.0.1\n    type: endpoint\n    criticality: 3\n    networkZone: unspecified", 1)
	p, err := Compile([]byte(explicit))
	if err != nil || p.Revision != base.Revision {
		t.Fatalf("explicit defaults changed identity: %v", err)
	}
	for _, bad := range []string{"priority: 0", "priority: 101", "priority: true", "jitter: 8s", "jitter: -1s", "enabled: yes", "unknown: 1"} {
		data := strings.Replace(string(b), "    - id: http\n", "    - id: http\n      "+bad+"\n", 1)
		if _, err := Compile([]byte(data)); err == nil {
			t.Errorf("invalid check field accepted: %s", bad)
		}
	}
	disabled := strings.Replace(string(b), "spec:\n", "spec:\n  enabled: false\n", 1)
	p, err = Compile([]byte(disabled))
	if err != nil || !p.Disabled || p.RegularAllowed("branch") {
		t.Fatal("disabled plan admitted regular work", err)
	}
	if err = VerifyPlan(p, "office"); err != nil {
		t.Fatal(err)
	}
	a, err := monitor.Evaluate(p, []monitor.Slot{{Agent: "branch", Zone: "office"}}, nil, time.Now())
	if err != nil || a.State != "unknown" || a.Zones[0].Reason != "monitor_disabled" {
		t.Fatal(a, err)
	}
	// A disabled optional check is validated but removed from executable/coverage IDs.
	extra := strings.Replace(string(b), "  assessment:", "    - id: optional\n      enabled: false\n      type: http\n      config: {url: 'http://127.0.0.1/'}\n  assessment:", 1)
	p, err = Compile([]byte(extra))
	if err != nil || len(p.Checks) != 1 || p.Revision != base.Revision {
		t.Fatal("disabled optional check changed active coverage", err)
	}
}

func TestUnweightedRulesRespectEvidenceGates(t *testing.T) {
	b, _ := os.ReadFile("../../examples/http.yaml")
	source := strings.Replace(string(b), "  assessment:", "  assessment:\n    weighted: {enabled: false}\n    defaultState: healthy", 1)
	p, err := Compile([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	slots := []monitor.Slot{{Agent: "branch", Zone: "office"}, {Agent: "dc", Zone: "server"}}
	proofs := []monitor.Observation{}
	for _, slot := range slots {
		proofs = append(proofs, monitor.Observation{ID: slot.Agent, Monitor: p.Name, Revision: p.Revision, Check: "http", Agent: slot.Agent, Zone: slot.Zone, StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "success"})
	}
	a, err := monitor.Evaluate(p, slots, proofs, now)
	if err != nil || a.State != "healthy" {
		t.Fatal(a, err)
	}
	missing, _ := monitor.Evaluate(p, slots, nil, now)
	if missing.State != "unknown" {
		t.Fatal("default healthy bypassed missing proofs", missing)
	}
	proofs[0].Outcome = "failure"
	negative, _ := monitor.Evaluate(p, slots, proofs, now)
	if negative.Zones[0].State != "down" {
		t.Fatal("default healthy bypassed required failure", negative)
	}
	if err = VerifyPlan(p, "office"); err != nil {
		t.Fatal(err)
	}
}

func TestDiagnosticHashCanonicalCollectionsWithoutMutatingInput(t *testing.T) {
	raw, _ := os.ReadFile("../../examples/diagnostics/fixed.yaml")
	p, err := CompileDiagnostics(raw)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(p)
	first, _ := DiagnosticProfileHash(p)
	q := CanonicalDiagnosticProfile(p)
	for i, j := 0, len(q.Actions)-1; i < j; i, j = i+1, j-1 {
		q.Actions[i], q.Actions[j] = q.Actions[j], q.Actions[i]
	}
	for i, j := 0, len(q.Steps)-1; i < j; i, j = i+1, j-1 {
		q.Steps[i], q.Steps[j] = q.Steps[j], q.Steps[i]
	}
	for i, j := 0, len(q.Hypotheses)-1; i < j; i, j = i+1, j-1 {
		q.Hypotheses[i], q.Hypotheses[j] = q.Hypotheses[j], q.Hypotheses[i]
	}
	second, _ := DiagnosticProfileHash(q)
	after, _ := json.Marshal(p)
	if first != second || string(before) != string(after) {
		t.Fatal("catalog hash depends on irrelevant order or mutates source")
	}
	q.Actions[0].Cost++
	changed, _ := DiagnosticProfileHash(q)
	if first == changed {
		t.Fatal("cost change lost from identity")
	}
}
