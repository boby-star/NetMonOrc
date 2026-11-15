package dependency

import "testing"

func TestRequiresGraphAndContext(t *testing.T) {
	m := Reference{Kind: "Monitor", Name: "web"}
	a := Reference{Kind: "Target", Name: "dns"}
	b := Reference{Kind: "Target", Name: "resolver"}
	edge := func(name string, from, to Reference, relation string) Edge {
		return Edge{Name: name, Spec: Spec{From: from, To: to, Relation: relation, Strength: "soft"}}
	}
	edges := []Edge{edge("web-dns", m, a, "requires"), edge("dns-resolver", a, b, "relatedTo"), edge("resolver-dns", b, a, "relatedTo")}
	known := map[string]bool{m.Key(): true, a.Key(): true, b.Key(): true}
	if err := ValidateGraph(edges, known); err != nil {
		t.Fatal(err)
	}
	context := Context(edges, m)
	if len(context) != 3 || context[0].Name != "dns-resolver" {
		t.Fatal("context missing cycle/determinism", context)
	}
	cycle := append(append([]Edge{}, edges...), edge("dns-web", a, m, "requires"))
	if err := ValidateGraph(cycle, known); err == nil {
		t.Fatal("requires cycle admitted")
	}
	unknown := Reference{Kind: "Target", Name: "missing"}
	if err := ValidateGraph([]Edge{edge("missing", m, unknown, "requires")}, known); err == nil {
		t.Fatal("missing reference admitted")
	}
	if err := ValidateGraph([]Edge{edge("self", m, m, "relatedTo")}, known); err == nil {
		t.Fatal("self relation admitted")
	}
	spec := Spec{From: m, To: a}
	if err := spec.Normalize(); err != nil || spec.Relation != "requires" || spec.Strength != "soft" {
		t.Fatal("defaults", err, spec)
	}
}
