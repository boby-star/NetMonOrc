package resources

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func assessmentFixtures(t *testing.T) (Manifest, Manifest) {
	t.Helper()
	policy, err := Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"AssessmentPolicy","metadata":{"name":"shared-policy"},"spec":{"requiredZones":["office"],"checks":[{"id":"http","weight":1,"required":true}],"rules":[{"id":"attempt-healthy","when":"true","then":{"state":"healthy","reason":"DECLARATIVE_RULE"}}],"weighted":{"enabled":true},"defaultState":"unknown"}}`))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Monitor","metadata":{"name":"service"},"spec":{"target":{"address":"127.0.0.1"},"checks":[{"id":"http","type":"http","runner":"native-http","interval":"10s","timeout":"1s","config":{"url":"http://127.0.0.1/"}}],"assessmentRef":"shared-policy"}}`))
	if err != nil {
		t.Fatal(err)
	}
	return policy, m
}
func TestAssessmentReferencePreservesRequiredFailureAndRevision(t *testing.T) {
	policy, m := assessmentFixtures(t)
	var weighted map[string]any
	_ = json.Unmarshal(policy.Spec, &weighted)
	weighted["checks"] = []any{map[string]any{"id": "http", "weight": .05, "required": true}, map[string]any{"id": "optional", "weight": .95}}
	policy.Spec, _ = json.Marshal(weighted)
	var source map[string]any
	_ = json.Unmarshal(m.Spec, &source)
	second := map[string]any{"id": "optional", "type": "http", "runner": "native-http", "interval": "10s", "timeout": "1s", "config": map[string]any{"url": "http://127.0.0.1/"}}
	source["checks"] = append(source["checks"].([]any), second)
	m.Spec, _ = json.Marshal(source)
	base := Registry{Active: "service", Zones: []string{"office"}}
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{m, policy})
	if err != nil {
		t.Fatal(err)
	}
	plan := *compiled["Monitor/service"].Monitor
	if plan.AssessmentRef != "shared-policy" || plan.AssessmentHash != compiled["AssessmentPolicy/shared-policy"].Revision {
		t.Fatal("policy not bound")
	}
	if err = config.VerifyPlan(plan, "office"); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	observation := monitor.Observation{ID: "negative", Monitor: plan.Name, Revision: plan.Revision, Agent: "branch", Zone: "office", Check: "http", StartedAt: now.Add(-time.Second), FinishedAt: now, Execution: "completed", Outcome: "failure"}
	positive := observation
	positive.ID = "positive"
	positive.Check = "optional"
	positive.Outcome = "success"
	assessment, err := monitor.Evaluate(plan, []monitor.Slot{{Agent: "branch", Zone: "office"}}, []monitor.Observation{observation, positive}, now)
	if err != nil || assessment.State != "down" {
		t.Fatal("policy cancelled a required failure", assessment.State, err)
	}
	if assessment.Zones[0].H == nil || *assessment.Zones[0].H < .94 {
		t.Fatal("test did not exercise high H with required failure")
	}
	missing, err := monitor.Evaluate(plan, []monitor.Slot{{Agent: "branch", Zone: "office"}}, nil, now)
	if err != nil || missing.State != "unknown" {
		t.Fatal("healthy CEL bypassed missing evidence", missing.State, err)
	}
	var spec map[string]any
	_ = json.Unmarshal(policy.Spec, &spec)
	spec["minCoverage"] = .5
	policy.Spec, _ = json.Marshal(spec)
	_, changed, err := ResolveCatalog(base, []Manifest{m}, []Manifest{policy})
	if err != nil || changed["Monitor/service"].Revision == plan.Revision {
		t.Fatal("policy dependency not in revision", err)
	}
}
func TestAssessmentOneOfMissingRefAndRosterDefaults(t *testing.T) {
	policy, m := assessmentFixtures(t)
	base := Registry{Active: "service", Zones: []string{"office"}}
	if _, _, err := ResolveCatalog(base, nil, []Manifest{m}); err == nil {
		t.Fatal("missing assessmentRef admitted")
	}
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	spec["assessment"] = map[string]any{}
	m.Spec, _ = json.Marshal(spec)
	if _, _, err := ResolveCatalog(base, nil, []Manifest{m, policy}); err == nil {
		t.Fatal("both assessment forms admitted")
	}
	delete(spec, "assessmentRef")
	delete(spec, "assessment")
	m.Spec, _ = json.Marshal(spec)
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{m})
	if err != nil {
		t.Fatal("approved assessment defaults rejected", err)
	}
	if len(compiled["Monitor/service"].Monitor.Zones) != 1 || compiled["Monitor/service"].Monitor.Checks[0].Required {
		t.Fatal("defaults invented required checks or zones")
	}
}
func TestPolicyTargetContextUsesActualDescriptorAndLabels(t *testing.T) {
	policy, m := assessmentFixtures(t)
	target := Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "Target", Metadata: Metadata{Name: "context-target", Labels: map[string]string{"site": "branch"}}, Spec: json.RawMessage(`{"type":"service","address":"127.0.0.1","networkZone":"server-room","criticality":4}`)}
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	delete(spec, "target")
	spec["targetRef"] = "context-target"
	m.Spec, _ = json.Marshal(spec)
	policy.Spec = json.RawMessage(`{"requiredZones":["office"],"checks":[{"id":"http","required":true}],"rules":[{"id":"context","when":"target.criticality == 4 && target.networkZone == 'server-room' && target.labels['site'] == 'branch'","then":{"state":"degraded","reason":"TARGET_CONTEXT"}}]}`)
	base := Registry{Active: "service", Zones: []string{"office"}}
	_, compiled, err := ResolveCatalog(base, nil, []Manifest{target, m, policy})
	if err != nil {
		t.Fatal(err)
	}
	plan := *compiled["Monitor/service"].Monitor
	now := time.Now()
	o := monitor.Observation{ID: "proof", Monitor: plan.Name, Revision: plan.Revision, Agent: "branch", Zone: "office", Check: "http", StartedAt: now.Add(-time.Second), FinishedAt: now, Execution: "completed", Outcome: "success"}
	a, err := monitor.Evaluate(plan, []monitor.Slot{{Agent: "branch", Zone: "office"}}, []monitor.Observation{o}, now)
	if err != nil || a.State != "degraded" {
		t.Fatal("CEL ignored actual Target context", a.State, err)
	}
	target.Metadata.Labels = map[string]string{"site": "other"}
	_, next, err := ResolveCatalog(base, nil, []Manifest{target, m, policy})
	if err != nil || next["Monitor/service"].Revision == plan.Revision {
		t.Fatal("Target label change not revisioned", err)
	}
}
