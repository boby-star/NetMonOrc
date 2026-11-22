package resources

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/dependency"
)

func shorthandFixture(t *testing.T) (Registry, Manifest, []Manifest) {
	t.Helper()
	data, err := os.ReadFile("../../examples/http.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := DecodeYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	targets := []Manifest{}
	for _, name := range []string{"resolver", "router"} {
		targets = append(targets, Manifest{APIVersion: m.APIVersion, Kind: "Target", Metadata: Metadata{Name: name}, Spec: json.RawMessage(`{"type":"logical"}`)})
	}
	return Registry{Active: m.Metadata.Name, Zones: []string{"office", "server"}}, m, targets
}
func withDependencies(m Manifest, raw string) Manifest {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(m.Spec, &fields)
	fields["dependencies"] = json.RawMessage(raw)
	m.Spec, _ = json.Marshal(fields)
	return m
}
func TestShorthandDependenciesCanonicalContextAndEmptyCompatibility(t *testing.T) {
	base, m, targets := shorthandFixture(t)
	compile := func(x Manifest) Compiled {
		t.Helper()
		_, out, err := ResolveCatalog(base, nil, append([]Manifest{x}, targets...))
		if err != nil {
			t.Fatal(err)
		}
		return out[Key(x)]
	}
	plain := compile(m)
	empty := compile(withDependencies(m, `[]`))
	if plain.Revision != empty.Revision {
		t.Fatal("empty shorthand changed legacy hash")
	}
	a := compile(withDependencies(m, `["resolver","router"]`))
	b := compile(withDependencies(m, `["router","resolver"]`))
	if a.Revision != b.Revision || a.Revision == plain.Revision || len(a.Monitor.Dependencies) != 2 {
		t.Fatal("noncanonical or missing shorthand context")
	}
	for _, edge := range a.Monitor.Dependencies {
		if edge.From != (dependency.Reference{Kind: "Monitor", Name: m.Metadata.Name}) || edge.Relation != "requires" || edge.Strength != "soft" {
			t.Fatal("invalid shorthand semantics", edge)
		}
	}
	s := New(prepareRepository{catalog: append([]Manifest{withDependencies(m, `["resolver"]`)}, targets...)}, base)
	view, err := s.MonitorDependencyContext(context.Background(), m.Metadata.Name)
	if err != nil || len(view.Items) != 1 || !view.Items[0].Derived || view.Items[0].DeclaredBy != Key(m) {
		t.Fatal("derived provenance unavailable", view, err)
	}
}
func TestShorthandDependencyRejectsBadRefsAndMixedCycles(t *testing.T) {
	base, m, targets := shorthandFixture(t)
	for _, bad := range []string{`null`, `{}`, `[null]`, `[1]`, `["resolver","resolver"]`, `["missing"]`, `["Monitor/research-web"]`} {
		if _, _, err := ResolveCatalog(base, nil, append([]Manifest{withDependencies(m, bad)}, targets...)); err == nil {
			t.Fatal("invalid shorthand accepted", bad)
		}
	}
	m = withDependencies(m, `["resolver"]`)
	back := Manifest{APIVersion: m.APIVersion, Kind: "Dependency", Metadata: Metadata{Name: "back"}, Spec: json.RawMessage(`{"from":{"kind":"Target","name":"resolver"},"to":{"kind":"Monitor","name":"research-web"}}`)}
	if _, _, err := ResolveCatalog(base, nil, append([]Manifest{m, back}, targets...)); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatal("mixed explicit/shorthand cycle accepted", err)
	}
}
func TestExplicitRequiresOverridesShorthandWithoutDuplicateVote(t *testing.T) {
	base, m, targets := shorthandFixture(t)
	explicit := Manifest{APIVersion: m.APIVersion, Kind: "Dependency", Metadata: Metadata{Name: "named-requirement"}, Spec: json.RawMessage(`{"from":{"kind":"Monitor","name":"research-web"},"to":{"kind":"Target","name":"resolver"},"strength":"hard"}`)}
	_, plain, err := ResolveCatalog(base, nil, append([]Manifest{m, explicit}, targets...))
	if err != nil {
		t.Fatal(err)
	}
	_, both, err := ResolveCatalog(base, nil, append([]Manifest{withDependencies(m, `["resolver"]`), explicit}, targets...))
	if err != nil {
		t.Fatal(err)
	}
	if plain[Key(m)].Revision != both[Key(m)].Revision || len(both[Key(m)].Monitor.Dependencies) != 1 {
		t.Fatal("duplicate implicit requirement changed explicit context")
	}
}
