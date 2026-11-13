package config

import (
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
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
	baseline, e := WithStaticComparison(resolved)
	if e != nil {
		t.Fatal(e)
	}
	if baseline.Revision == resolved.Revision || !baseline.Diagnostics.ComparisonStatic {
		t.Fatal("comparison mode did not change effective revision")
	}
	if e = VerifyPlan(baseline, "office"); e != nil {
		t.Fatal(e)
	}
	if resolved.Diagnostics.ComparisonStatic {
		t.Fatal("comparison mutated original binding")
	}
	profile.Actions[0].Cost = 2
	changed, e := ResolveDiagnostics(p, profile)
	if e != nil {
		t.Fatal(e)
	}
	if changed.Revision == resolved.Revision {
		t.Fatal("profile dependency change did not change effective revision")
	}
	profile.Actions[0].Cost = 1
	profile.Actions[0].Retry = &diagnostics.Retry{MaxAttempts: 2}
	retried, e := ResolveDiagnostics(p, profile)
	if e != nil || retried.Revision == resolved.Revision {
		t.Fatal("retry policy did not change effective revision", e)
	}
	profile.Actions[0].AttemptNo = 2
	dispatched, e := ResolveDiagnostics(p, profile)
	if e != nil || dispatched.Revision != retried.Revision {
		t.Fatal("dispatch ordinal changed declarative revision", e)
	}
}
