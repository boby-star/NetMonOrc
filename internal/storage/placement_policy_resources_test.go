package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func testPlacementPolicyResources(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
	request := func(method, path string, body any, version string) (int, []byte) {
		t.Helper()
		var input io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			input = bytes.NewReader(b)
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
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, b
	}
	policy := resources.Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "PlacementPolicy", Metadata: resources.Metadata{Name: "placement-test"}, Spec: json.RawMessage(`{"strategy":"fixed","agentRefs":["test-agent"],"replicas":1}`)}
	m, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	m.Metadata.Name = "placement-monitor"
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	spec["placementRef"] = "placement-test"
	m.Spec, _ = json.Marshal(spec)
	batch := resources.Batch{Manifests: []resources.Manifest{m, policy}, RequestID: "placement-unordered"}
	code, data := request("POST", "/api/v1/config/plan", batch, "")
	var preview resources.Plan
	if code != 200 || json.Unmarshal(data, &preview) != nil || len(preview.Placements) != 1 || len(preview.Placements[0].SelectedAgents) != 1 || preview.Placements[0].Pending {
		t.Fatalf("planned selection absent %d %s", code, data)
	}
	code, data = request("POST", "/api/v1/config/apply", batch, "")
	if code != 202 {
		t.Fatalf("placement mixed apply %d %s", code, data)
	}
	before, found, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || !found {
		t.Fatal(err)
	}
	registry := resources.Registry{Active: "service", Zones: []string{"office"}, AlertChannel: "default-webhook", Roster: []monitor.Slot{{Agent: "test-agent", Zone: "office"}}}
	_, compiled, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, registry)
	if err != nil || compiled.Revision != before.Revision {
		t.Fatal("placement JSONB round trip", err)
	}
	if err = config.VerifyPlan(*compiled.Monitor, "office", "test-agent"); err != nil {
		t.Fatal(err)
	}
	current, _, err := s.ResourceKind(ctx, "PlacementPolicy", "placement-test")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/PlacementPolicy/placement-test", nil, current.ResourceVersion); code != 409 {
		t.Fatal("referenced placement deleted", code)
	}
	policy.Spec = json.RawMessage(`{"strategy":"fixed","agentRefs":["test-agent"],"replicas":1,"zones":["unavailable-zone"]}`)
	version := current.ResourceVersion
	batch = resources.Batch{Manifests: []resources.Manifest{policy}, ExpectedVersions: map[string]*string{"PlacementPolicy/placement-test": &version}, RequestID: "placement-pending"}
	code, data = request("POST", "/api/v1/config/apply", batch, "")
	if code != 202 {
		t.Fatalf("pending placement rejected %d %s", code, data)
	}
	var op resources.Operation
	_ = json.Unmarshal(data, &op)
	after, _, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || after.Revision == before.Revision || after.ResourceVersion != before.ResourceVersion || len(op.Result.Targets) != 2 {
		t.Fatal("placement dependency/version invariant", err)
	}
	_, pending, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, registry)
	if err != nil || len(pending.Monitor.FixedPlacement.Selected) != 0 {
		t.Fatal("partial placement created", err)
	}
	if err = s.ReconcileOperations(ctx, m.Metadata.Name, pending.Revision, nil, nil); err != nil {
		t.Fatal(err)
	}
	state, _, err := s.Operation(ctx, op.ID)
	if err != nil || state.State == "completed" {
		t.Fatal("pending placement falsely completed", err)
	}
	foundReason := false
	for _, target := range state.Result.Targets {
		if target.Kind == "Monitor" && target.Reason == "insufficient_placement_agents" {
			foundReason = true
		}
	}
	if !foundReason {
		t.Fatal("pending reason not exposed")
	}
	original, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	original.Metadata.Name = m.Metadata.Name
	if code, _ = request("PUT", "/api/v1/resources/Monitor/placement-monitor", original, after.ResourceVersion); code != 200 {
		t.Fatal("unlink placement", code)
	}
	current, _, err = s.ResourceKind(ctx, "PlacementPolicy", "placement-test")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/PlacementPolicy/placement-test", nil, current.ResourceVersion); code != 204 {
		t.Fatal("unreferenced placement deletion", code)
	}
}
