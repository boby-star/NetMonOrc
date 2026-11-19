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
	for _, spec := range []string{`{"strategy":"not-a-strategy"}`, `{"strategy":"fixed","agentRefs":["branch"],"onInsufficientAgents":"partial"}`, `{"strategy":"fixed","agentRefs":["branch"],"selector":{"matchExpressions":[{"key":"role","operator":"Bad"}]}}`, `{"strategy":"fixed","agentRefs":["branch"],"distinctBy":"unknown"}`, `{"strategy":"fixed","agentRefs":["branch","branch"]}`} {
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

func TestDynamicPlacementWithdrawsCachedSelectionAfterTargetChange(t *testing.T) {
	base, policy, m := fixedPlacementFixture(t)
	policy.Spec = json.RawMessage(`{"strategy":"leastLoaded","agentRefs":["branch","dc"],"zones":["office"]}`)
	registry, compiled, err := ResolveCatalog(base, nil, []Manifest{policy, m})
	if err != nil {
		t.Fatal(err)
	}
	pending := compiled["Monitor/service"].Monitor
	base.ResolvedPlacements = map[string]monitor.ResolvedPlacement{"service": {PolicyHash: registry.Placements["fixed-placement"].Hash(), TemplateHash: pending.Revision, Selected: []monitor.Slot{{Agent: "branch", Zone: "office"}}}}
	_, selected, err := ResolveCatalog(base, nil, []Manifest{policy, m})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected["Monitor/service"].Monitor.FixedPlacement.Selected) != 1 {
		t.Fatal("valid cached selection lost")
	}
	var spec map[string]any
	if err = json.Unmarshal(m.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	checks := spec["checks"].([]any)
	checks[0].(map[string]any)["config"].(map[string]any)["url"] = "http://198.18.0.80/"
	m.Spec, _ = json.Marshal(spec)
	_, changed, err := ResolveCatalog(base, nil, []Manifest{policy, m})
	if err != nil {
		t.Fatal(err)
	}
	if len(changed["Monitor/service"].Monitor.FixedPlacement.Selected) != 0 {
		t.Fatal("old target authorization reused for changed configuration")
	}
}

func TestSameZonePlacementAndDistinctZoneConstraint(t *testing.T) {
	base, policy, m := fixedPlacementFixture(t)
	base.Zones = []string{"office"}
	base.Roster[1].Zone = "office"
	policy.Spec = json.RawMessage(`{"strategy":"fixed","agentRefs":["branch","dc"],"replicas":2}`)
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{m, policy})
	if err != nil {
		t.Fatal(err)
	}
	plan := *compiled["Monitor/service"].Monitor
	if len(plan.Zones) != 1 || len(plan.FixedPlacement.Selected) != 2 {
		t.Fatal("same-zone sources not independently planned", plan)
	}
	if err = config.VerifyPlan(plan, "office", "dc"); err != nil {
		t.Fatal(err)
	}
	policy.Spec = json.RawMessage(`{"strategy":"fixed","agentRefs":["branch","dc"],"replicas":2,"distinctBy":"zone"}`)
	_, compiled, err = ResolveCatalog(base, nil, []Manifest{m, policy})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled["Monitor/service"].Monitor.FixedPlacement.Selected) != 0 {
		t.Fatal("distinct zones fabricated")
	}
}

func TestTrustedSelectorAndFailureDomainPlacement(t *testing.T) {
	base, policy, m := fixedPlacementFixture(t)
	base.AgentLabels = map[string]map[string]string{"branch": {"role": "probe", "site": "office"}, "dc": {"role": "probe", "site": "server"}}
	base.FailureDomains = map[string]string{"branch": "uplink-a", "dc": "uplink-b"}
	policy.Spec = json.RawMessage(`{"strategy":"fixed","replicas":2,"distinctBy":"failureDomain","selector":{"matchLabels":{"role":"probe"},"matchExpressions":[{"key":"site","operator":"In","values":["office","server"]}]}}`)
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{m, policy})
	if err != nil {
		t.Fatal(err)
	}
	p := *compiled["Monitor/service"].Monitor
	if len(p.FixedPlacement.Selected) != 2 || p.FixedPlacement.FailureDomains["branch"] != "uplink-a" {
		t.Fatal("trusted domain set not selected", p.FixedPlacement)
	}
	if err = config.VerifyPlan(p, "office", "branch"); err != nil {
		t.Fatal(err)
	}
	revision := p.Revision
	base.FailureDomains["dc"] = "uplink-a"
	_, compiled, err = ResolveCatalog(base, nil, []Manifest{m, policy})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled["Monitor/service"].Monitor.FixedPlacement.Selected) != 0 || revision == compiled["Monitor/service"].Revision {
		t.Fatal("shared domains kept stale assignment")
	}
	delete(base.FailureDomains, "dc")
	_, compiled, err = ResolveCatalog(base, nil, []Manifest{m, policy})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled["Monitor/service"].Monitor.FixedPlacement.Selected) != 0 {
		t.Fatal("unknown domain inferred from zone")
	}
	base.FailureDomains["dc"] = "uplink-b"
	base.AgentLabels["dc"]["role"] = "other"
	_, compiled, err = ResolveCatalog(base, nil, []Manifest{m, policy})
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled["Monitor/service"].Monitor.FixedPlacement.Selected) != 0 {
		t.Fatal("selector bypassed for replicas")
	}
}

func TestSelectorCanonicalizationAndInventoryValidation(t *testing.T) {
	a, err := compilePlacement(json.RawMessage(`{"selector":{"matchExpressions":[{"key":"site","operator":"In","values":["server","office"]},{"key":"role","operator":"Exists"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := compilePlacement(json.RawMessage(`{"selector":{"matchExpressions":[{"key":"role","operator":"Exists"},{"key":"site","operator":"In","values":["office","server"]}]}}`))
	if err != nil || a.Hash() != b.Hash() {
		t.Fatal("selector order changed policy hash", err)
	}
	base, _, _ := fixedPlacementFixture(t)
	for _, raw := range []string{`{"absent":{"role":"probe"}}`, `{"branch":{"role":"probe","role":"other"}}`, `{"branch":null}`, `{"branch":{"Role":"probe"}}`} {
		if _, err = ParseAgentLabels([]byte(raw), base.Roster); err == nil {
			t.Fatal("invalid inventory accepted", raw)
		}
	}
	if _, err = ParseAgentLabels([]byte(`{"branch":{"role":"probe"},"dc":{}}`), base.Roster); err != nil {
		t.Fatal(err)
	}
}
