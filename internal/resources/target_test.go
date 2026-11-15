package resources

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/config"
)

func TestTargetGraphAndAgentRevision(t *testing.T) {
	target := Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "Target", Metadata: Metadata{Name: "shared-target"}, Spec: json.RawMessage(`{"type":"host","address":"127.0.0.1","attributes":{"ready":false,"depth":0}}`)}
	monitor, err := Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Monitor","metadata":{"name":"service"},"spec":{"targetRef":"shared-target","checks":[{"id":"tcp","type":"tcp","interval":"10s","timeout":"1s","config":{"port":443}}],"assessment":{"requiredZones":["office"]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	base := Registry{Active: "service", Zones: []string{"office"}}
	if _, _, err = ResolveCatalog(base, nil, []Manifest{monitor}); err == nil {
		t.Fatal("missing targetRef admitted")
	}
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{monitor, target})
	if err != nil {
		t.Fatal(err)
	}
	attrs := compiled["Target/shared-target"].Target.Attributes
	if len(attrs) != 2 || attrs["ready"].Boolean || attrs["depth"].Integer != 0 {
		t.Fatal("false/zero target attributes lost")
	}
	plan := *compiled["Monitor/service"].Monitor
	if plan.TargetAddress != "127.0.0.1" || plan.TargetRef != "shared-target" || plan.TargetHash != compiled["Target/shared-target"].Revision {
		t.Fatal("target not resolved")
	}
	if err = config.VerifyPlan(plan, "office"); err != nil {
		t.Fatal("resolved plan rejected by Agent", err)
	}
	target.Spec = json.RawMessage(`{"type":"host","address":"127.0.0.1","criticality":4}`)
	_, next, err := ResolveCatalog(base, []Manifest{monitor}, []Manifest{target})
	if err != nil {
		t.Fatal(err)
	}
	if next["Monitor/service"].Revision == plan.Revision {
		t.Fatal("target-only change did not change dependent revision")
	}
	var spec map[string]any
	_ = json.Unmarshal(monitor.Spec, &spec)
	spec["target"] = map[string]string{"address": "127.0.0.1"}
	monitor.Spec, _ = json.Marshal(spec)
	if _, _, err = ResolveCatalog(base, nil, []Manifest{monitor, target}); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatal("both target and targetRef admitted", err)
	}
}
func TestTargetClosedSchemaAndPassiveLogical(t *testing.T) {
	base := Registry{}
	for _, spec := range []string{`{"type":"host"}`, `{"type":"host","address":"https://example.com"}`, `{"type":"host","address":"bad host"}`, `{"type":"host","address":"999.999.999.999"}`, `{"type":"logical","criticality":0}`, `{"type":"logical","criticality":3.0}`, `{"type":"logical","attributes":{"bad":[]}}`, `{"type":"logical","address":null}`, `{"type":"logical","unknown":1}`} {
		m := Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "Target", Metadata: Metadata{Name: "passive"}, Spec: json.RawMessage(spec)}
		if _, err := base.CompileResource(m); err == nil {
			t.Fatal("invalid target admitted", spec)
		}
	}
	m := Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "Target", Metadata: Metadata{Name: "passive"}, Spec: json.RawMessage(`{"type":"logical"}`)}
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{m})
	if err != nil || len(compiled) != 1 || compiled["Target/passive"].Monitor != nil {
		t.Fatal("passive target created a Monitor", err)
	}
	if compiled["Target/passive"].Target.Criticality != 3 || compiled["Target/passive"].Target.NetworkZone != "unspecified" {
		t.Fatal("target defaults differ from DSL")
	}
}

func TestTargetHostnameKeepsAbsoluteDNSName(t *testing.T) {
	for _, input := range []string{"API.Example.COM", "API.Example.COM."} {
		address, err := normalizeTargetAddress(input)
		if err != nil || address != strings.ToLower(input) {
			t.Fatal("hostname resolution semantics changed", input, address, err)
		}
	}
}
