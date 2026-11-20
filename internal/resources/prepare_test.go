package resources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type prepareRepository struct {
	Repository
	catalog []Manifest
	current map[string]View
}

func (r prepareRepository) Catalog(context.Context) ([]Manifest, error) { return r.catalog, nil }
func (r prepareRepository) ResourceKind(_ context.Context, kind, name string) (View, bool, error) {
	v, found := r.current[kind+"/"+name]
	return v, found, nil
}

func TestPreparePreservesTypedNumbersAndExplicitConditions(t *testing.T) {
	p, m := profileFixture(t)
	m.Spec = json.RawMessage(strings.Replace(string(m.Spec), `"url":"https://127.0.0.1/"`, `"url":"https://127.0.0.1/","ratio":1.0`, 1))
	base := Registry{Active: "service", Zones: []string{"office"}}
	repo := prepareRepository{current: map[string]View{"Monitor/service": {ResourceVersion: "uid:7"}, "CheckProfile/reusable-http": {ResourceVersion: "profile:2"}}}
	s := New(repo, base)
	batch := Batch{Manifests: []Manifest{m, p}, ExpectedVersions: map[string]*string{"CheckProfile/reusable-http": nil}}
	prepared, err := s.Prepare(context.Background(), batch)
	if err != nil {
		t.Fatal(err)
	}
	if _, mutated := batch.ExpectedVersions["Monitor/service"]; mutated {
		t.Fatal("caller preconditions mutated")
	}
	if !strings.Contains(prepared.Source, `"ratio": 1.0`) {
		t.Fatal("float argument lost numeric type")
	}
	decoded, err := DecodeBatch([]byte(prepared.Source))
	if err != nil {
		t.Fatal(err)
	}
	if *decoded.ExpectedVersions["Monitor/service"] != "uid:7" || decoded.ExpectedVersions["CheckProfile/reusable-http"] != nil {
		t.Fatal("version or explicit create-only condition overwritten")
	}
	_, before, err := ResolveCatalog(base, nil, batch.Manifests)
	if err != nil {
		t.Fatal(err)
	}
	_, after, err := ResolveCatalog(base, nil, decoded.Manifests)
	if err != nil || before["Monitor/service"].Revision != after["Monitor/service"].Revision {
		t.Fatal("prepare changed effective hash", err)
	}
	again, err := s.Prepare(context.Background(), decoded)
	if err != nil || again.Source != prepared.Source {
		t.Fatal("reprepare changed request or body", err)
	}
	stale := "uid:3"
	decoded.ExpectedVersions["Monitor/service"] = &stale
	preserved, err := s.Prepare(context.Background(), decoded)
	if err != nil || !strings.Contains(preserved.Source, `"uid:3"`) {
		t.Fatal("stale explicit precondition refreshed", err)
	}
	decoded.ExpectedVersions["Target/typo"] = nil
	if _, err = s.Prepare(context.Background(), decoded); err == nil {
		t.Fatal("foreign precondition accepted")
	}
}
