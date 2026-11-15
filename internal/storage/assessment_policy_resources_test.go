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
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func testAssessmentPolicyResources(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
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
	policy, err := resources.Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"AssessmentPolicy","metadata":{"name":"policy-test"},"spec":{"minCoverage":1e0,"freshness":{"maxSkew":"30s"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	m, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	m.Metadata.Name = "policy-monitor"
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	delete(spec, "assessment")
	spec["assessmentRef"] = "policy-test"
	m.Spec, _ = json.Marshal(spec)
	code, data := request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{m, policy}, RequestID: "policy-unordered"}, "")
	if code != 202 {
		t.Fatalf("policy mixed apply: %d %s", code, data)
	}
	before, found, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || !found {
		t.Fatal(err)
	}
	registry := resources.Registry{Active: "service", Zones: []string{"office"}, AlertChannel: "default-webhook"}
	_, compiled, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, registry)
	if err != nil || compiled.Revision != before.Revision || compiled.Monitor.AssessmentRef != "policy-test" {
		t.Fatal("policy normalization changed after JSONB round trip", err)
	}
	if err = config.VerifyPlan(*compiled.Monitor, "office"); err != nil {
		t.Fatal(err)
	}
	current, found, err := s.ResourceKind(ctx, "AssessmentPolicy", "policy-test")
	if err != nil || !found {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/AssessmentPolicy/policy-test", nil, current.ResourceVersion); code != 409 {
		t.Fatal("referenced policy deleted", code)
	}
	if code, _ = request("PUT", "/api/v1/resources/AssessmentPolicy/policy-test", policy, ""); code != 428 {
		t.Fatal("missing policy precondition", code)
	}
	if err = s.SaveAssessment(ctx, monitor.Assessment{Monitor: m.Metadata.Name, Revision: before.Revision, At: time.Now(), State: "healthy"}); err != nil {
		t.Fatal(err)
	}
	policy.Spec = json.RawMessage(`{"zoneAggregation":"any","minCoverage":0.5,"rules":[{"id":"planned-degradation","when":"true","then":{"state":"degraded","reason":"POLICY_TEST"}}]}`)
	version := current.ResourceVersion
	code, data = request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{policy}, ExpectedVersions: map[string]*string{"AssessmentPolicy/policy-test": &version}, RequestID: "policy-only"}, "")
	if code != 202 {
		t.Fatalf("policy-only apply %d %s", code, data)
	}
	var op resources.Operation
	if json.Unmarshal(data, &op) != nil || len(op.Result.Targets) != 2 {
		t.Fatal("missing implicit policy target")
	}
	after, _, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || after.Revision == before.Revision || after.ResourceVersion != before.ResourceVersion {
		t.Fatal("policy dependency invariant", err)
	}
	if _, found, err = s.ReadLiveAssessment(ctx, m.Metadata.Name, before.Revision); err != nil || found {
		t.Fatal("old policy healthy assessment returned as live", err)
	}
	_, anyPlan, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, registry)
	if err != nil || anyPlan.Monitor.ZoneAggregation != "any" {
		t.Fatal("zone any lost after JSONB policy update", err)
	}
	if err = config.VerifyPlan(*anyPlan.Monitor, "office"); err != nil {
		t.Fatal(err)
	}
	original, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	original.Metadata.Name = m.Metadata.Name
	if code, _ = request("PUT", "/api/v1/resources/Monitor/policy-monitor", original, after.ResourceVersion); code != 200 {
		t.Fatal("unlink policy", code)
	}
	current, _, err = s.ResourceKind(ctx, "AssessmentPolicy", "policy-test")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/AssessmentPolicy/policy-test", nil, current.ResourceVersion); code != 204 {
		t.Fatal("unreferenced policy delete", code)
	}
}
