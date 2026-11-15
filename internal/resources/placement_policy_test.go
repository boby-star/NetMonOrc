package resources

import (
	"encoding/json"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func fixedPlacementFixture(t *testing.T) (Registry, Manifest, Manifest) {
	t.Helper()
	base := Registry{Active: "service", Zones: []string{"office", "server"}, Roster: []monitor.Slot{{Agent: "branch", Zone: "office"}, {Agent: "dc", Zone: "server"}}}
	policy := Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "PlacementPolicy", Metadata: Metadata{Name: "fixed-placement"}, Spec: json.RawMessage(`{"strategy":"fixed","agentRefs":["branch","dc"],"replicas":1,"zones":["office"],"distinctBy":"zone"}`)}
	m, err := Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Monitor","metadata":{"name":"service"},"spec":{"target":{"address":"127.0.0.1"},"placementRef":"fixed-placement","checks":[{"id":"http","type":"http","runner":"native-http","interval":"10s","timeout":"1s","config":{"url":"http://127.0.0.1/"}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	return base, policy, m
}
func TestFixedPlacementAuthorizesSelectedAndIdleAgent(t *testing.T) {
	base, policy, m := fixedPlacementFixture(t)
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{m, policy})
	if err != nil {
		t.Fatal(err)
	}
	p := *compiled["Monitor/service"].Monitor
	if len(p.FixedPlacement.Selected) != 1 || !p.RegularAllowed("branch") || p.RegularAllowed("dc") || len(p.Zones) != 1 || p.Zones[0] != "office" {
		t.Fatal("fixed selection/default zones incorrect")
	}
	if err = config.VerifyPlan(p, "office", "branch"); err != nil {
		t.Fatal("selected Agent rejected", err)
	}
	if err = config.VerifyPlan(p, "server", "dc"); err != nil {
		t.Fatal("idle Agent cannot receive an authenticated plan", err)
	}
	if err = config.VerifyPlan(p, "office", "dc"); err == nil {
		t.Fatal("Agent zone forgery accepted")
	}
	if len(p.RegularSlots(base.Roster)) != 1 || len(p.AssessmentSlots(base.Roster)) != 1 {
		t.Fatal("regular/assessment slots not filtered")
	}
}
func TestFixedPlacementPendingDoesNotAuthorizePartialProbes(t *testing.T) {
	base, policy, m := fixedPlacementFixture(t)
	policy.Spec = json.RawMessage(`{"strategy":"fixed","agentRefs":["branch","dc"],"replicas":2,"zones":["office"]}`)
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	spec["assessment"] = map[string]any{"requiredZones": []string{"office"}}
	m.Spec, _ = json.Marshal(spec)
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{policy, m})
	if err != nil {
		t.Fatal(err)
	}
	p := *compiled["Monitor/service"].Monitor
	if p.RegularAllowed("branch") || p.RegularAllowed("dc") || len(p.RegularSlots(base.Roster)) != 0 {
		t.Fatal("partial set authorized while pending")
	}
	if err = config.VerifyPlan(p, "server", "dc"); err != nil {
		t.Fatal(err)
	}
}
func TestFixedPlacementRejectsUnsupportedStrategiesAndBadReferences(t *testing.T) {
	base, policy, m := fixedPlacementFixture(t)
	for _, spec := range []string{`{"strategy":"leastLoaded"}`, `{"strategy":"roundRobin"}`, `{"strategy":"fixed","agentRefs":["branch"],"onInsufficientAgents":"partial"}`, `{"strategy":"fixed","agentRefs":["branch"],"selector":{"matchLabels":{"role":"probe"}}}`, `{"strategy":"fixed","agentRefs":["branch"],"distinctBy":"failureDomain"}`, `{"strategy":"fixed","agentRefs":["branch","branch"]}`} {
		invalid := policy
		invalid.Spec = json.RawMessage(spec)
		if _, err := base.CompileResource(invalid); err == nil {
			t.Fatal("unsupported placement admitted", spec)
		}
	}
	policy.Spec = json.RawMessage(`{"strategy":"fixed","agentRefs":["absent"]}`)
	if _, _, err := ResolveCatalog(base, nil, []Manifest{m, policy}); err == nil {
		t.Fatal("unregistered Agent ref admitted")
	}
}
