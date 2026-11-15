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
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func testCheckProfileResources(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
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
	profile, err := resources.Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"CheckProfile","metadata":{"name":"checks-test"},"spec":{"parameters":[{"name":"url","type":"url","required":true},{"name":"expected","type":"integer","default":200},{"name":"ratio","type":"float","default":1e100}],"checks":[{"id":"http","type":"http","runner":"native-http","interval":"10s","timeout":"1s","config":{"url":"${params.url}","expectedStatus":["${params.expected}"]}}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	m, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	m.Metadata.Name = "profile-monitor"
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	delete(spec, "checks")
	spec["useProfiles"] = []any{map[string]any{"ref": "checks-test", "as": "web", "parameters": map[string]any{"url": "http://127.0.0.1:18080/"}}}
	m.Spec, _ = json.Marshal(spec)
	code, data := request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{m, profile}, RequestID: "checks-unordered"}, "")
	if code != 202 {
		t.Fatalf("CheckProfile mixed apply: %d %s", code, data)
	}
	before, found, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || !found {
		t.Fatal(err)
	}
	registry := resources.Registry{Active: "service", Zones: []string{"office"}, AlertChannel: "default-webhook"}
	_, compiled, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, registry)
	if err != nil || compiled.Revision != before.Revision || compiled.Monitor.Checks[0].ID != "web__http" {
		t.Fatal("profile revision changed after JSONB round trip", err)
	}
	if err = config.VerifyPlan(*compiled.Monitor, "office"); err != nil {
		t.Fatal(err)
	}
	current, found, err := s.ResourceKind(ctx, "CheckProfile", "checks-test")
	if err != nil || !found {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/CheckProfile/checks-test", nil, current.ResourceVersion); code != 409 {
		t.Fatal("referenced CheckProfile deleted", code)
	}
	if code, _ = request("PUT", "/api/v1/resources/CheckProfile/checks-test", profile, ""); code != 428 {
		t.Fatal("missing profile precondition", code)
	}
	var profileSpec map[string]any
	decoder := json.NewDecoder(bytes.NewReader(profile.Spec))
	decoder.UseNumber()
	_ = decoder.Decode(&profileSpec)
	profileSpec["parameters"].([]any)[1].(map[string]any)["default"] = 503
	profile.Spec, _ = json.Marshal(profileSpec)
	version := current.ResourceVersion
	code, data = request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{profile}, ExpectedVersions: map[string]*string{"CheckProfile/checks-test": &version}, RequestID: "checks-only"}, "")
	if code != 202 {
		t.Fatalf("profile-only apply %d %s", code, data)
	}
	var op resources.Operation
	if err = json.Unmarshal(data, &op); err != nil || len(op.Result.Targets) != 2 {
		t.Fatal("missing implicit Monitor", err)
	}
	after, _, err := s.Resource(ctx, m.Metadata.Name)
	if err != nil || before.Revision == after.Revision || before.ResourceVersion != after.ResourceVersion {
		t.Fatal("profile dependency/version invariant", err)
	}
	_, updated, err := s.LoadCompiledMonitor(ctx, m.Metadata.Name, registry)
	if err != nil || updated.Revision != after.Revision || updated.Monitor.Checks[0].ExpectedStatus[0] != 503 {
		t.Fatal("bound profile value did not reach IR", err)
	}
	original, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	original.Metadata.Name = m.Metadata.Name
	if code, _ = request("PUT", "/api/v1/resources/Monitor/profile-monitor", original, after.ResourceVersion); code != 200 {
		t.Fatal("unlink profile", code)
	}
	current, _, err = s.ResourceKind(ctx, "CheckProfile", "checks-test")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/CheckProfile/checks-test", nil, current.ResourceVersion); code != 204 {
		t.Fatal("delete unreferenced profile", code)
	}
}
