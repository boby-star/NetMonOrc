package resources

import (
	"encoding/json"
	"testing"
)

func TestDiagnosticPlacementReferenceResolutionAndRevision(t *testing.T) {
	policy, err := DecodeYAML([]byte("apiVersion: netmonorc.io/v1alpha1\nkind: PlacementPolicy\nmetadata: {name: probe-agents}\nspec: {selector: {matchLabels: {role: probe}}, strategy: leastLoaded, zones: [office]}\n"))
	if err != nil {
		t.Fatal(err)
	}
	profile, err := Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"DiagnosticProfile","metadata":{"name":"ref-profile"},"spec":{"mode":"fixed","budget":{"maxAdditionalChecks":1,"maxDuration":"10s"},"hypotheses":[{"id":"fault"}],"actions":[{"id":"inspect","check":{"type":"http","timeout":"1s","config":{"url":"http://192.0.2.1/"}},"placementRef":"probe-agents"}],"fixed":{"steps":[{"id":"first","actionRef":"inspect"}]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	base := Registry{}
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{profile, policy})
	if err != nil {
		t.Fatal(err)
	}
	action := compiled["DiagnosticProfile/ref-profile"].Profile.Actions[0]
	if action.Placement.Ref != "probe-agents" || action.Placement.Hash == "" || action.Placement.Selector == nil {
		t.Fatal(action.Placement)
	}
	if _, _, err := ResolveCatalog(base, nil, []Manifest{profile}); err == nil {
		t.Fatal("dangling placementRef admitted")
	}
	var spec map[string]any
	json.Unmarshal(policy.Spec, &spec)
	spec["zones"] = []string{"server"}
	policy.Spec, _ = json.Marshal(spec)
	_, changed, err := ResolveCatalog(base, []Manifest{profile}, []Manifest{policy})
	if err != nil || changed["DiagnosticProfile/ref-profile"].Revision == compiled["DiagnosticProfile/ref-profile"].Revision {
		t.Fatal("policy update did not change profile revision", err)
	}
}
