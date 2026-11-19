package resources

import (
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

func checkPlacementFixture(t *testing.T) (Registry, Manifest) {
	t.Helper()
	base, _, _ := fixedPlacementFixture(t)
	base.ExecuteAll = true
	m, err := Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Monitor","metadata":{"name":"service"},"spec":{"target":{"address":"127.0.0.1"},"checks":[{"id":"left","type":"http","runner":"native-http","interval":"10s","timeout":"1s","config":{"url":"http://127.0.0.1/"},"placement":{"strategy":"leastLoaded","agentRefs":["branch"],"zones":["office"]}},{"id":"right","type":"http","runner":"native-http","interval":"10s","timeout":"1s","config":{"url":"http://127.0.0.1/"},"placement":{"strategy":"fixed","agentRefs":["dc"],"zones":["server"]}}],"assessment":{"checks":[{"id":"left","weight":1,"required":true},{"id":"right","weight":1,"required":true}]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return base, m
}
func TestPerCheckSourcesCoverageAndCacheWithdrawal(t *testing.T) {
	base, m := checkPlacementFixture(t)
	registry, catalog, err := ResolveCatalog(base, nil, []Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	p := *catalog["Monitor/service"].Monitor
	if len(p.CheckPlacements) != 2 || p.CheckAllowed("left", "branch") || !p.CheckAllowed("right", "dc") || p.CheckAllowed("right", "branch") {
		t.Fatal("per-check ownership not enforced", p)
	}
	template, err := registry.PlacementTemplate(m)
	if err != nil {
		t.Fatal(err)
	}
	base.ResolvedPlacements = map[string]monitor.ResolvedPlacement{"service/left": {PolicyHash: p.CheckPlacements["left"].Hash, TemplateHash: template.Revision, Selected: []monitor.Slot{{Agent: "branch", Zone: "office"}}}}
	_, catalog, err = ResolveCatalog(base, nil, []Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	p = *catalog["Monitor/service"].Monitor
	if !p.CheckAllowed("left", "branch") || p.CheckAllowed("left", "dc") {
		t.Fatal("cached check selection not scoped")
	}
	for _, slot := range base.Roster {
		if err = config.VerifyPlan(p, slot.Zone, slot.Agent); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	proofs := []monitor.Observation{}
	for _, v := range []struct{ check, agent, zone string }{{"left", "branch", "office"}, {"right", "dc", "server"}} {
		proofs = append(proofs, monitor.Observation{ID: v.check, Monitor: p.Name, Revision: p.Revision, Check: v.check, Agent: v.agent, Zone: v.zone, StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "success"})
	}
	a, err := monitor.Evaluate(p, p.AssessmentSlots(base.Roster), proofs, now)
	if err != nil || a.State != "healthy" || len(a.Zones) != 2 {
		t.Fatal("different check scopes counted as missing replicas", a, err)
	}
	for _, z := range a.Zones {
		if z.Q != 1 || z.Coverage != "complete" || len(z.Evidence) != 1 {
			t.Fatal("coverage included unplanned check/Agent pairs", z)
		}
	}
	forged := proofs[0]
	forged.Agent = "dc"
	forged.Zone = "server"
	proofs[0] = forged
	a, err = monitor.Evaluate(p, p.AssessmentSlots(base.Roster), proofs, now)
	if err != nil || a.State == "healthy" {
		t.Fatal("wrong check source supplied healthy evidence", a, err)
	}
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	spec["checks"].([]any)[0].(map[string]any)["config"].(map[string]any)["url"] = "http://192.0.2.1/"
	m.Spec, _ = json.Marshal(spec)
	_, catalog, err = ResolveCatalog(base, nil, []Manifest{m})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog["Monitor/service"].Monitor.CheckPlacements["left"].Selected) != 0 {
		t.Fatal("old per-check target admission resurrected")
	}
}
func TestCheckPlacementOneOfAndUnknownRef(t *testing.T) {
	for _, spec := range []string{`{"id":"x","placement":{},"placementRef":"other"}`, `{"id":"x","placement":{"replicas":0}}`, `{"id":"x","placementRef":false}`} {
		if _, _, err := splitCheckPlacement(json.RawMessage(spec)); err == nil {
			t.Fatal("invalid placement accepted", spec)
		}
	}
	base, m := checkPlacementFixture(t)
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	c := spec["checks"].([]any)[0].(map[string]any)
	delete(c, "placement")
	c["placementRef"] = "missing"
	m.Spec, _ = json.Marshal(spec)
	if _, _, err := ResolveCatalog(base, nil, []Manifest{m}); err == nil {
		t.Fatal("unknown per-check policy ref accepted")
	}
}
