package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func testTargetResources(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
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
	target := resources.Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "Target", Metadata: resources.Metadata{Name: "target-resource"}, Spec: json.RawMessage(`{"type":"host","address":"127.0.0.1"}`)}
	m, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	m.Metadata.Name = "target-monitor"
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	delete(spec, "target")
	spec["targetRef"] = "target-resource"
	m.Spec, _ = json.Marshal(spec)
	code, data := request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{m, target}, RequestID: "target-unordered"}, "")
	if code != 202 {
		t.Fatalf("mixed targetRef apply: %d %s", code, data)
	}
	before, found, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || !found {
		t.Fatal(err)
	}
	current, found, err := s.ResourceKind(ctx, "Target", target.Metadata.Name)
	if err != nil || !found {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/Target/target-resource", nil, current.ResourceVersion); code != 409 {
		t.Fatal("referenced target deleted", code)
	}
	if err = s.SaveAssessment(ctx, monitor.Assessment{Monitor: m.Metadata.Name, Revision: before.Revision, At: time.Now(), State: "healthy"}); err != nil {
		t.Fatal(err)
	}
	target.Spec = json.RawMessage(`{"type":"host","address":"127.0.0.1","criticality":4}`)
	version := current.ResourceVersion
	code, data = request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{target}, ExpectedVersions: map[string]*string{"Target/target-resource": &version}, RequestID: "target-only"}, "")
	if code != 202 {
		t.Fatalf("target-only apply %d %s", code, data)
	}
	var op resources.Operation
	if err = json.Unmarshal(data, &op); err != nil || len(op.Result.Targets) != 2 {
		t.Fatal("missing implicit target", err)
	}
	after, found, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || !found || after.Revision == before.Revision || after.ResourceVersion != before.ResourceVersion {
		t.Fatal("dependent revision/manifest version semantics", err)
	}
	if _, found, err = s.ReadLiveAssessment(ctx, m.Metadata.Name, before.Revision); err != nil || found {
		t.Fatal("old target proof returned as live", err)
	}
	// Unlink before deleting the target; source Manifest/version remains independent.
	_ = json.Unmarshal(m.Spec, &spec)
	delete(spec, "targetRef")
	spec["target"] = map[string]string{"address": "127.0.0.1"}
	m.Spec, _ = json.Marshal(spec)
	if code, _ = request("PUT", "/api/v1/resources/Monitor/target-monitor", m, after.ResourceVersion); code != 200 {
		t.Fatal("unlink target", code)
	}
	current, _, err = s.ResourceKind(ctx, "Target", target.Metadata.Name)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/Target/target-resource", nil, current.ResourceVersion); code != 204 {
		t.Fatal("unreferenced target delete", code)
	}
}
