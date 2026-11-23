package resources

import (
	"testing"
)

func TestSelectorAlertReferencesValidateOnlyMatchingMonitors(t *testing.T) {
	decode := func(source string) Manifest {
		t.Helper()
		m, err := DecodeYAML([]byte(source))
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	one := decode("apiVersion: netmonorc.io/v1alpha1\nkind: Monitor\nmetadata: {name: one, labels: {environment: research}}\nspec: {target: {address: 192.0.2.1}, checks: [{id: http, type: http, interval: 10s, timeout: 1s, config: {url: 'http://192.0.2.1/'}}], assessment: {requiredZones: [office]}}\n")
	two := decode("apiVersion: netmonorc.io/v1alpha1\nkind: Monitor\nmetadata: {name: two, labels: {environment: other}}\nspec: {target: {address: 192.0.2.1}, checks: [{id: other, type: http, interval: 10s, timeout: 1s, config: {url: 'http://192.0.2.1/'}}], assessment: {requiredZones: [office]}}\n")
	rule := decode("apiVersion: netmonorc.io/v1alpha1\nkind: AlertRule\nmetadata: {name: selected}\nspec: {selector: {matchLabels: {environment: research}}, scope: zone, when: 'checks.http.outcome == \"failure\"'}\n")
	base := Registry{Zones: []string{"office"}, AlertChannel: "default-webhook"}
	if _, _, err := ResolveCatalog(base, nil, []Manifest{one, two, rule}); err != nil {
		t.Fatal(err)
	}
	for _, expr := range []string{`zone.checks.missing.outcome == "failure"`, `zones.office.checks.missing.outcome == "failure"`} {
		bad := decode("apiVersion: netmonorc.io/v1alpha1\nkind: AlertRule\nmetadata: {name: bad}\nspec: {monitorRef: one, scope: zone, when: '" + expr + "'}\n")
		if _, _, err := ResolveCatalog(base, nil, []Manifest{one, bad}); err == nil {
			t.Fatalf("unknown check accepted: %s", expr)
		}
	}
	two.Metadata.Labels = map[string]string{"environment": "research"}
	if _, _, err := ResolveCatalog(base, nil, []Manifest{one, two, rule}); err == nil {
		t.Fatal("newly matching Monitor with missing check accepted")
	}
}
