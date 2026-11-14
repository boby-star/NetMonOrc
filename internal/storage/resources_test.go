package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/httpapi"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

const testMonitor = `{"apiVersion":"netmonorc.io/v1alpha1","kind":"Monitor","metadata":{"name":"service"},"spec":{"target":{"address":"127.0.0.1"},"checks":[{"id":"http","type":"http","runner":"native-http","interval":"10s","timeout":"1s","config":{"url":"http://127.0.0.1:18080/","expectedStatus":[200]}}],"assessment":{"requiredZones":["office"]}}}`

func configureResourceTest(t *testing.T, ctx context.Context, s *Store, h *httpapi.Handler) {
	t.Helper()
	m, e := resources.Decode([]byte(testMonitor))
	if e != nil {
		t.Fatal(e)
	}
	registry := resources.Registry{Active: "service", Zones: []string{"office"}}
	p, e := registry.Compile(m)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.SeedResource(ctx, m, p.Revision); e != nil {
		t.Fatal(e)
	}
	h.SetResources(resources.New(s, registry), "")
}
func testResourceUseCases(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
	bearerToken := ""
	request := func(method, path string, body any, etag, csrf string) (int, []byte, string) {
		t.Helper()
		var input io.Reader
		if body != nil {
			b, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			input = bytes.NewReader(b)
		}
		r, e := http.NewRequest(method, server.URL+path, input)
		if e != nil {
			t.Fatal(e)
		}
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		r.Header.Set("Origin", server.URL)
		if bearerToken != "" {
			r.Header.Set("Authorization", "Bearer "+bearerToken)
		}
		if etag != "" {
			r.Header.Set("If-Match", etag)
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, data, response.Header.Get("ETag")
	}
	code, data, activeTag := request("GET", "/api/v1/resources/Monitor/service", nil, "", "")
	var active resources.View
	if code != 200 || activeTag == "" || json.Unmarshal(data, &active) != nil || active.UID == "" || active.Generation != 1 {
		t.Fatal("initial managed resource", code)
	}
	if code, _, _ := request("DELETE", "/api/v1/resources/Monitor/service", nil, activeTag, session.CSRFToken); code != 409 {
		t.Fatal("active Monitor deleted", code)
	}
	m := active.Manifest
	m.Metadata.Name = "standby"
	if code, _, _ := request("POST", "/api/v1/resources/Monitor", m, "", ""); code != 403 {
		t.Fatal("resource write without CSRF", code)
	}
	parsed, _ := url.Parse(server.URL)
	rawSession := ""
	for _, cookie := range client.Jar.Cookies(parsed) {
		if cookie.Name == "netmonorc_session" {
			rawSession = cookie.Value
		}
	}
	writer, e := auth.New(s).IssueToken(ctx, rawSession, auth.APITokenCreate{Name: "resource-writer", Scopes: []string{"resources:write"}})
	if e != nil {
		t.Fatal(e)
	}
	bearerToken = writer.Token
	code, data, tag := request("POST", "/api/v1/resources/Monitor", m, "", session.CSRFToken)
	var saved resources.View
	if code != 201 || tag == "" || json.Unmarshal(data, &saved) != nil || saved.Status.State != "pending" {
		t.Fatal("Monitor create", code)
	}
	if code, _, _ := request("GET", "/api/v1/resources/Monitor/standby", nil, "", ""); code != 403 {
		t.Fatal("write-only PAT bypassed read scope", code)
	}
	bearerToken = ""
	firstUID := saved.UID
	if code, _, _ := request("POST", "/api/v1/resources/Monitor", m, "", session.CSRFToken); code != 409 {
		t.Fatal("duplicate resource", code)
	}
	if code, _, _ := request("PUT", "/api/v1/resources/Monitor/standby", m, "", session.CSRFToken); code != 428 {
		t.Fatal("missing resource If-Match", code)
	}
	m.Metadata.Labels = map[string]string{"env": "research"}
	code, data, metadataTag := request("PUT", "/api/v1/resources/Monitor/standby", m, tag, session.CSRFToken)
	if code != 200 || metadataTag == tag || json.Unmarshal(data, &saved) != nil || saved.Generation != 1 {
		t.Fatal("metadata-only generation", code)
	}
	var spec map[string]any
	if e := json.Unmarshal(m.Spec, &spec); e != nil {
		t.Fatal(e)
	}
	spec["checks"].([]any)[0].(map[string]any)["config"].(map[string]any)["expectedStatus"] = []int{503}
	m.Spec, _ = json.Marshal(spec)
	code, data, specTag := request("PUT", "/api/v1/resources/Monitor/standby", m, metadataTag, session.CSRFToken)
	if code != 200 || json.Unmarshal(data, &saved) != nil || saved.Generation != 2 {
		t.Fatal("spec generation", code)
	}
	if code, _, _ := request("PUT", "/api/v1/resources/Monitor/standby", m, metadataTag, session.CSRFToken); code != 412 {
		t.Fatal("stale resource update", code)
	}
	code, data, _ = request("GET", "/api/v1/resources/Monitor?labelSelector=env=research", nil, "", "")
	var page resources.Page
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].Manifest.Metadata.Name != "standby" {
		t.Fatal("resource label filter", code)
	}
	code, data, _ = request("POST", "/api/v1/config/validate", resources.Batch{Manifests: []resources.Manifest{m}}, "", session.CSRFToken)
	var validation resources.Validation
	if code != 200 || json.Unmarshal(data, &validation) != nil || !validation.Valid {
		t.Fatal("configuration validation", code)
	}
	code, data, _ = request("POST", "/api/v1/config/plan", resources.Batch{Manifests: []resources.Manifest{m}}, "", session.CSRFToken)
	var plan resources.Plan
	if code != 200 || json.Unmarshal(data, &plan) != nil || plan.PlanID == "" || len(plan.Diffs) != 1 || plan.Diffs[0].Action != "unchanged" {
		t.Fatal("configuration dry run", code)
	}
	invalid := m
	spec["checks"].([]any)[0].(map[string]any)["interval"] = "1ms"
	invalid.Spec, _ = json.Marshal(spec)
	code, data, _ = request("POST", "/api/v1/config/validate", resources.Batch{Manifests: []resources.Manifest{invalid}}, "", session.CSRFToken)
	if code != 200 || json.Unmarshal(data, &validation) != nil || validation.Valid || len(validation.Errors) == 0 {
		t.Fatal("invalid batch accepted", code)
	}
	if code, _, _ := request("PUT", "/api/v1/resources/Monitor/standby", invalid, specTag, session.CSRFToken); code != 422 {
		t.Fatal("invalid manifest persisted", code)
	}
	code, data, _ = request("GET", "/api/v1/resources/Monitor/standby/revisions?limit=2", nil, "", "")
	var history struct {
		Items      []resources.Revision `json:"items"`
		NextCursor *string              `json:"nextCursor"`
	}
	if code != 200 || json.Unmarshal(data, &history) != nil || len(history.Items) != 2 || history.NextCursor == nil {
		t.Fatal("resource history", code)
	}
	code, data, _ = request("GET", "/api/v1/resources/Monitor/standby/revisions?limit=2&cursor="+url.QueryEscape(*history.NextCursor), nil, "", "")
	if code != 200 || json.Unmarshal(data, &history) != nil || len(history.Items) != 1 || history.NextCursor != nil {
		t.Fatal("history cursor", code)
	}
	if code, _, _ := request("DELETE", "/api/v1/resources/Monitor/standby", nil, specTag, session.CSRFToken); code != 204 {
		t.Fatal("standby delete", code)
	}
	if code, _, _ := request("GET", "/api/v1/resources/Monitor/standby", nil, "", ""); code != 404 {
		t.Fatal("deleted resource visible", code)
	}
	code, data, _ = request("POST", "/api/v1/resources/Monitor", m, "", session.CSRFToken)
	if code != 201 || json.Unmarshal(data, &saved) != nil || saved.UID == firstUID {
		t.Fatal("recreated identity", code)
	}
	if code, _, _ := request("PUT", "/api/v1/resources/Monitor/standby", m, specTag, session.CSRFToken); code != 412 {
		t.Fatal("old identity ETag reused", code)
	}
	if code, _, _ := request("GET", "/api/v1/resources/Target", nil, "", ""); code != 422 {
		t.Fatal("unsupported kind advertised", code)
	}
	// A fresh persisted result is still ineligible if it belongs to a former desired revision.
	stored, _, e := s.Resource(ctx, "service")
	if e != nil {
		t.Fatal(e)
	}
	oldRevision := stored.Revision
	if e = s.SaveAssessment(ctx, monitor.Assessment{Monitor: "service", Revision: oldRevision, At: time.Now(), State: "healthy"}); e != nil {
		t.Fatal(e)
	}
	if _, found, e := s.ReadLiveAssessment(ctx, "service", oldRevision); e != nil || !found {
		t.Fatal("live gate rejected current assessment", e)
	}
	newActive := m
	newActive.Metadata = resources.Metadata{Name: "service"}
	code, data, newActiveTag := request("PUT", "/api/v1/resources/Monitor/service", newActive, activeTag, session.CSRFToken)
	if code != 200 || json.Unmarshal(data, &saved) != nil {
		t.Fatal("active resource replace", code)
	}
	if _, found, e := s.ReadLiveAssessment(ctx, "service", oldRevision); e != nil || found {
		t.Fatal("old healthy leaked during reconciliation", e)
	}
	if _, found, e := s.ReadLiveAssessment(ctx, "service", saved.Status.DesiredRevision); e != nil || found {
		t.Fatal("old snapshot relabeled as new revision", e)
	}
	if code, _, _ := request("PUT", "/api/v1/resources/Monitor/service", active.Manifest, newActiveTag, session.CSRFToken); code != 200 {
		t.Fatal("restore active resource", code)
	}

}
