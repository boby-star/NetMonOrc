package resources

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/config"
)

func profileFixture(t *testing.T) (Manifest, Manifest) {
	t.Helper()
	profile, err := DecodeYAML([]byte(`apiVersion: netmonorc.io/v1alpha1
kind: CheckProfile
metadata: {name: reusable-http}
spec:
  parameters:
    - {name: url, type: url, required: true}
    - {name: expected, type: integer, default: 200}
    - {name: verify, type: boolean, default: true}
    - {name: ratio, type: float, default: 1.0}
  checks:
    - id: http
      type: http
      runner: native-http
      interval: 10s
      timeout: 1s
      config: {url: '${params.url}', expectedStatus: ['${params.expected}'], verifyTLS: '${params.verify}'}
`))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Monitor","metadata":{"name":"service"},"spec":{"target":{"address":"127.0.0.1"},"useProfiles":[{"ref":"reusable-http","as":"web","parameters":{"url":"https://127.0.0.1/"}}],"assessment":{"requiredZones":["office"],"checks":[{"id":"web__http","weight":1,"required":true}]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return profile, m
}
func TestCheckProfileExpansionAndCanonicalRevision(t *testing.T) {
	profile, m := profileFixture(t)
	base := Registry{Active: "service", Zones: []string{"office"}}
	_, all, err := ResolveCatalog(base, nil, []Manifest{m, profile})
	if err != nil {
		t.Fatal(err)
	}
	plan := *all["Monitor/service"].Monitor
	if len(plan.Checks) != 1 || plan.Checks[0].ID != "web__http" || plan.Checks[0].ExpectedStatus[0] != 200 || len(plan.CheckProfiles) != 1 {
		t.Fatal("profile expansion differs from DSL")
	}
	if err = config.VerifyPlan(plan, "office"); err != nil {
		t.Fatal("Agent rejected profile plan", err)
	}
	// Source object key order and JSONB numeric formatting cannot alter revision.
	var raw map[string]any
	d := json.NewDecoder(strings.NewReader(string(profile.Spec)))
	d.UseNumber()
	if err = d.Decode(&raw); err != nil {
		t.Fatal(err)
	}
	raw["parameters"].([]any)[3].(map[string]any)["default"] = json.Number("1e0")
	profile.Spec, _ = json.Marshal(raw)
	_, again, err := ResolveCatalog(base, nil, []Manifest{profile, m})
	if err != nil || again["Monitor/service"].Revision != plan.Revision {
		t.Fatal("unstable normalized profile revision", err)
	}
	raw["description"] = "changed template"
	profile.Spec, _ = json.Marshal(raw)
	_, changed, err := ResolveCatalog(base, nil, []Manifest{profile, m})
	if err != nil || changed["Monitor/service"].Revision == plan.Revision {
		t.Fatal("profile dependency not part of revision", err)
	}
}
func TestProfileBindingRejectsMissingWrongAndCollidingParameters(t *testing.T) {
	p, m := profileFixture(t)
	base := Registry{Active: "service", Zones: []string{"office"}}
	for _, usage := range []string{
		`[{"ref":"reusable-http","as":"web"}]`,
		`[{"ref":"reusable-http","as":"web","parameters":{"url":"https://127.0.0.1/","expected":"200"}}]`,
		`[{"ref":"reusable-http","as":"web","parameters":{"url":"https://127.0.0.1/","unexpected":1}}]`,
		`[{"ref":"absent","as":"web"}]`,
		`[{"ref":"reusable-http","as":"` + strings.Repeat("a", 64) + `","parameters":{"url":"https://127.0.0.1/"}}]`,
		`[{"ref":"reusable-http","as":"web","parameters":{"url":"https://127.0.0.1/"}},{"ref":"reusable-http","as":"web","parameters":{"url":"https://127.0.0.1/"}}]`,
	} {
		var spec map[string]json.RawMessage
		_ = json.Unmarshal(m.Spec, &spec)
		spec["useProfiles"] = json.RawMessage(usage)
		bad := m
		bad.Spec, _ = json.Marshal(spec)
		if _, _, err := ResolveCatalog(base, nil, []Manifest{p, bad}); err == nil {
			t.Fatal("invalid profile usage accepted", usage)
		}
	}
}
func TestProfileRejectsPartialAndOutOfConfigTemplates(t *testing.T) {
	p, _ := profileFixture(t)
	for _, replacement := range []struct{ from, to string }{
		{"${params.url}", "https://example.com/${params.url}"},
		{`"interval":"10s"`, `"interval":"${params.url}"`},
		{"${params.url}", "${params.unknown}"},
		{`"expectedStatus":["${params.expected}"]`, `"expectedStatus":[99]`},
	} {
		bad := p
		bad.Spec = []byte(strings.ReplaceAll(string(p.Spec), replacement.from, replacement.to))
		if _, err := (Registry{}).CompileResource(bad); err == nil {
			t.Fatal("invalid template admitted", replacement.to)
		}
	}
}
func TestYAMLJSONPreservesNumbersAndRejectsAmbiguity(t *testing.T) {
	p, _ := profileFixture(t)
	compiled, err := (Registry{}).CompileResource(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(compiled.CheckProfile.Parameters[1].Default) != "1.0" { // sorted: expected, ratio, url, verify
		t.Fatal("YAML float identity lost", string(compiled.CheckProfile.Parameters[1].Default))
	}
	for _, text := range []string{
		"apiVersion: netmonorc.io/v1alpha1\nkind: Target\nmetadata: {name: a}\nspec: {type: logical, type: host}",
		"apiVersion: netmonorc.io/v1alpha1\nkind: Target\nmetadata: {name: a}\nspec: {type: logical, attributes: {v: .nan}}",
		"apiVersion: netmonorc.io/v1alpha1\nkind: Target\nmetadata: {name: a}\nspec: &loop {type: logical, attributes: {v: *loop}}",
	} {
		if _, err := DecodeYAML([]byte(text)); err == nil {
			t.Fatal("ambiguous or cyclic YAML admitted")
		}
	}
}

func TestDeclaredFloatSurvivesJSONBIntegralNotation(t *testing.T) {
	for _, raw := range []string{"1e100", "1" + strings.Repeat("0", 100)} {
		value, err := parameterValue("float", json.RawMessage(raw))
		if err != nil || value.Kind != "float" || value.Number != 1e100 {
			t.Fatal("numeric float normalization", err)
		}
	}
	if _, err := parameterValue("float", json.RawMessage(`"1.0"`)); err == nil {
		t.Fatal("string coerced to float")
	}
	if _, err := parameterValue("float", json.RawMessage("1e999")); err == nil {
		t.Fatal("nonfinite float accepted")
	}
}

func TestProfileExpansionIsBoundedBeforeIRCompilation(t *testing.T) {
	p, m := profileFixture(t)
	var spec map[string]json.RawMessage
	_ = json.Unmarshal(p.Spec, &spec)
	var checks []map[string]any
	for i := 0; i < 50; i++ {
		checks = append(checks, map[string]any{"id": fmt.Sprintf("http_%d", i), "type": "http", "runner": "native-http", "interval": "10s", "timeout": "1s", "config": map[string]any{"url": "${params.url}"}})
	}
	spec["checks"], _ = json.Marshal(checks)
	p.Spec, _ = json.Marshal(spec)
	var monitorSpec map[string]json.RawMessage
	_ = json.Unmarshal(m.Spec, &monitorSpec)
	monitorSpec["useProfiles"], _ = json.Marshal([]any{map[string]any{"ref": "reusable-http", "as": "web", "parameters": map[string]any{"url": "https://127.0.0.1/" + strings.Repeat("a", 30000)}}})
	delete(monitorSpec, "assessment")
	monitorSpec["assessment"] = json.RawMessage(`{"requiredZones":["office"]}`)
	m.Spec, _ = json.Marshal(monitorSpec)
	if _, _, err := ResolveCatalog(Registry{}, nil, []Manifest{p, m}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatal("expanded byte budget not enforced", err)
	}
}

func TestResourceJSONRejectsDuplicateEnvelopeAndNestedKeys(t *testing.T) {
	for _, data := range []string{
		`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Target","kind":"Monitor","metadata":{"name":"x"},"spec":{}}`,
		`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Target","metadata":{"name":"x","name":"y"},"spec":{}}`,
		`{"apiVersion":"netmonorc.io/v1alpha1","kind":"Target","metadata":{"name":"x"},"spec":{"type":"host","type":"logical"}}`,
	} {
		if _, err := Decode([]byte(data)); err == nil {
			t.Fatal("duplicate envelope/spec accepted", data)
		}
	}
}
