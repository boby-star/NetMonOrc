package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

// One end-to-end configuration use case exercises the transactional dependency
// boundary: changing a profile must never leave an old live Monitor revision.
func testProfileResources(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
	request := func(method, path string, body any, version string) (int, []byte) {
		t.Helper()
		var input io.Reader
		if body != nil {
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			input = bytes.NewReader(data)
		}
		req, err := http.NewRequest(method, server.URL+path, input)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-CSRF-Token", session.CSRFToken)
		if version != "" {
			req.Header.Set("If-Match", `"`+version+`"`)
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, data
	}
	data, err := os.ReadFile("../../deployment/lab/adaptive-confirmed.yaml")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := resources.DecodeYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	profile.Metadata.Name = "profile-test"
	m, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	m.Metadata.Name = profile.Metadata.Name
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	spec["diagnostics"] = map[string]any{"enabled": true, "profileRef": profile.Metadata.Name}
	m.Spec, _ = json.Marshal(spec)
	batch := resources.Batch{Manifests: []resources.Manifest{m, profile}, RequestID: "profile-mixed"}
	code, data := request("POST", "/api/v1/config/apply", batch, "")
	if code != 202 {
		t.Fatalf("unordered mixed batch: %d %s", code, data)
	}
	before, found, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || !found {
		t.Fatal("missing dependent", err)
	}
	p, found, err := s.ResourceKind(ctx, "DiagnosticProfile", profile.Metadata.Name)
	if err != nil || !found {
		t.Fatal("missing profile", err)
	}
	if before.UID == p.UID {
		t.Fatal("kind identities collided")
	}
	if code, _ = request("DELETE", "/api/v1/resources/DiagnosticProfile/"+profile.Metadata.Name, nil, p.ResourceVersion); code != 409 {
		t.Fatal("enabled reference deleted", code)
	}
	// Mark an imported profile; a later deletion must survive bootstrap recovery.
	if _, err = s.SeedResource(ctx, profile, p.Revision); err != nil {
		t.Fatal(err)
	}
	var profileSpec map[string]any
	_ = json.Unmarshal(profile.Spec, &profileSpec)
	profileSpec["actions"].([]any)[0].(map[string]any)["costUnits"] = 2
	profile.Spec, _ = json.Marshal(profileSpec)
	batch = resources.Batch{Manifests: []resources.Manifest{profile}, ExpectedVersions: map[string]*string{resources.Key(profile): &p.ResourceVersion}, RequestID: "profile-update"}
	code, data = request("POST", "/api/v1/config/apply", batch, "")
	var operation resources.Operation
	if code != 202 || json.Unmarshal(data, &operation) != nil || len(operation.Result.Targets) != 2 {
		t.Fatalf("dependent apply target: %d %s", code, data)
	}
	after, _, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || after.Revision == before.Revision || after.ResourceVersion != before.ResourceVersion || after.Generation != before.Generation {
		t.Fatal("dependency revision invariant", err)
	}
	implicit := false
	for _, target := range operation.Result.Targets {
		if target.Kind == "Monitor" && target.Implicit && target.EffectiveRevision == after.Revision {
			implicit = true
		}
	}
	if !implicit {
		t.Fatal("dependent ACK omitted")
	}
	if err = s.ReconcileOperations(ctx, "service", "other", nil, nil); err != nil {
		t.Fatal(err)
	}
	progress, _, err := s.Operation(ctx, operation.ID)
	if err != nil || progress.State == "completed" {
		t.Fatal("profile completed before dependent ACK", err)
	}
	// Startup resolves the database catalog rather than an old local profile.
	_, loaded, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, resources.Registry{})
	if err != nil || loaded.Revision != after.Revision {
		t.Fatal("restart lost database profile", err)
	}
	if code, _ = request("PUT", "/api/v1/resources/DiagnosticProfile/"+profile.Metadata.Name, profile, p.ResourceVersion); code != 412 {
		t.Fatal("stale profile accepted", code)
	}
	spec["diagnostics"].(map[string]any)["enabled"] = false
	m.Spec, _ = json.Marshal(spec)
	code, data = request("PUT", "/api/v1/resources/Monitor/"+m.Metadata.Name, m, after.ResourceVersion)
	if code != 200 {
		t.Fatalf("disable dependency: %d %s", code, data)
	}
	disabled, _, _ := s.Resource(ctx, m.Metadata.Name)
	p, _, _ = s.ResourceKind(ctx, "DiagnosticProfile", profile.Metadata.Name)
	if code, _ = request("DELETE", "/api/v1/resources/DiagnosticProfile/"+profile.Metadata.Name, nil, p.ResourceVersion); code != 204 {
		t.Fatal("unused profile delete", code)
	}
	deleted, _, _ := s.Resource(ctx, m.Metadata.Name)
	if deleted.Revision == disabled.Revision {
		t.Fatal("deleted optional dependency did not change effective hash")
	}
	if _, err = s.SeedResource(ctx, profile, p.Revision); !errors.Is(err, resources.ErrNotFound) {
		t.Fatal("deleted bootstrap profile resurrected", err)
	}
	spec["diagnostics"].(map[string]any)["enabled"] = true
	m.Spec, _ = json.Marshal(spec)
	if code, _ = request("PUT", "/api/v1/resources/Monitor/"+m.Metadata.Name, m, deleted.ResourceVersion); code != 422 {
		t.Fatal("missing enabled profile accepted", code)
	}
	if code, _ = request("DELETE", "/api/v1/resources/Monitor/"+m.Metadata.Name, nil, deleted.ResourceVersion); code != 204 {
		t.Fatal("test cleanup", code)
	}
}
