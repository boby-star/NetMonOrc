package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testDependencyResources(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
	request := func(method, path string, body any, version string) (int, []byte) {
		t.Helper()
		var in io.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			in = bytes.NewReader(raw)
		}
		r, _ := http.NewRequest(method, server.URL+path, in)
		r.Header.Set("Origin", server.URL)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", session.CSRFToken)
		if version != "" {
			r.Header.Set("If-Match", `"`+version+`"`)
		}
		resp, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, raw
	}
	m, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	m.Metadata.Name = "dependency-monitor"
	target := resources.Manifest{APIVersion: m.APIVersion, Kind: "Target", Metadata: resources.Metadata{Name: "dependency-target"}, Spec: json.RawMessage(`{"type":"logical"}`)}
	d := resources.Manifest{APIVersion: m.APIVersion, Kind: "Dependency", Metadata: resources.Metadata{Name: "declared-link"}, Spec: json.RawMessage(`{"from":{"kind":"Monitor","name":"dependency-monitor"},"to":{"kind":"Target","name":"dependency-target"}}`)}
	code, raw := request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{d, m, target}, RequestID: "dependency-mixed"}, "")
	if code != 202 {
		t.Fatalf("mixed dependency apply %d %s", code, raw)
	}
	before, _, _ := s.Resource(ctx, m.Metadata.Name)
	registry := resources.Registry{Active: "service", Zones: []string{"office"}, AlertChannel: "default-webhook"}
	_, compiled, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, registry)
	if err != nil || len(compiled.Monitor.Dependencies) != 1 || compiled.Monitor.Dependencies[0].Strength != "soft" {
		t.Fatal("context JSONB/defaults", err)
	}
	if err = config.VerifyPlan(*compiled.Monitor, "office"); err != nil {
		t.Fatal(err)
	}
	reverse := d
	reverse.Metadata.Name = "cycle-link"
	reverse.Spec = json.RawMessage(`{"from":{"kind":"Target","name":"dependency-target"},"to":{"kind":"Monitor","name":"dependency-monitor"}}`)
	if code, raw := request("POST", "/api/v1/resources/Dependency", reverse, ""); code != 422 {
		t.Fatalf("cycle accepted %d %s", code, raw)
	}
	if _, found, _ := s.ResourceKind(ctx, "Dependency", "cycle-link"); found {
		t.Fatal("cycle committed")
	}
	tr, _, _ := s.ResourceKind(ctx, "Target", target.Metadata.Name)
	if code, _ := request("DELETE", "/api/v1/resources/Target/"+target.Metadata.Name, nil, tr.ResourceVersion); code != 409 {
		t.Fatal("referenced Target deleted", code)
	}
	if code, _ := request("DELETE", "/api/v1/resources/Monitor/"+m.Metadata.Name, nil, before.ResourceVersion); code != 409 {
		t.Fatal("referenced Monitor deleted", code)
	}
	current, _, _ := s.ResourceKind(ctx, "Dependency", d.Metadata.Name)
	d.Spec = json.RawMessage(`{"from":{"kind":"Monitor","name":"dependency-monitor"},"to":{"kind":"Target","name":"dependency-target"},"strength":"hard"}`)
	if code, _ := request("PUT", "/api/v1/resources/Dependency/"+d.Metadata.Name, d, ""); code != 428 {
		t.Fatal("missing precondition", code)
	}
	if code, raw := request("PUT", "/api/v1/resources/Dependency/"+d.Metadata.Name, d, current.ResourceVersion); code != 200 {
		t.Fatalf("dependency update %d %s", code, raw)
	}
	after, _, _ := s.Resource(ctx, m.Metadata.Name)
	if after.Revision == before.Revision || after.ResourceVersion != before.ResourceVersion {
		t.Fatal("dependency did not fence affected Monitor")
	}
	current, _, _ = s.ResourceKind(ctx, "Dependency", d.Metadata.Name)
	if code, _ := request("DELETE", "/api/v1/resources/Dependency/"+d.Metadata.Name, nil, current.ResourceVersion); code != 204 {
		t.Fatal("unlink dependency", code)
	}
	_, plain, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, registry)
	if err != nil || len(plain.Monitor.Dependencies) != 0 {
		t.Fatal("context survived deletion", err)
	}
	after, _, _ = s.Resource(ctx, m.Metadata.Name)
	if code, _ := request("DELETE", "/api/v1/resources/Monitor/"+m.Metadata.Name, nil, after.ResourceVersion); code != 204 {
		t.Fatal("Monitor cleanup", code)
	}
	if code, _ := request("DELETE", "/api/v1/resources/Target/"+target.Metadata.Name, nil, tr.ResourceVersion); code != 204 {
		t.Fatal("Target cleanup", code)
	}
}
