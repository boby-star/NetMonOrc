package config

import (
	"os"
	"testing"
)

func TestResolvedDiagnosticDependencyChangesRevision(t *testing.T) {
	b, e := os.ReadFile("../../examples/http.yaml")
	if e != nil {
		t.Fatal(e)
	}
	b = append(b, []byte("\n  diagnostics:\n    enabled: true\n    profileRef: live-web\n")...)
	p, e := Compile(b)
	if e != nil {
		t.Fatal(e)
	}
	b, e = os.ReadFile("../../examples/diagnostics/live-adaptive.yaml")
	if e != nil {
		t.Fatal(e)
	}
	profile, e := CompileDiagnostics(b)
	if e != nil {
		t.Fatal(e)
	}
	resolved, e := ResolveDiagnostics(p, profile)
	if e != nil {
		t.Fatal(e)
	}
	if resolved.Revision == p.Revision {
		t.Fatal("unresolved dependency hash reused")
	}
	if e = VerifyPlan(resolved, "office"); e != nil {
		t.Fatal(e)
	}
	if e = VerifyPlan(p, "office"); e == nil {
		t.Fatal("unresolved Agent plan accepted")
	}
	profile.Actions[0].Cost = 2
	changed, e := ResolveDiagnostics(p, profile)
	if e != nil {
		t.Fatal(e)
	}
	if changed.Revision == resolved.Revision {
		t.Fatal("profile dependency change did not change effective revision")
	}
}
