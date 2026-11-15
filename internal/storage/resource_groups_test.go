package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testResourceGroups(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
	request := func(method, path string, body any, version string) (int, []byte) {
		t.Helper()
		var in io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			in = bytes.NewReader(b)
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
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, b
	}
	a, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	a.Metadata.Name = "group-a"
	b := a
	b.Metadata.Name = "group-b"
	g := resources.Manifest{APIVersion: a.APIVersion, Kind: "ResourceGroup", Metadata: resources.Metadata{Name: "test-group"}, Spec: json.RawMessage(`{"members":["group-b","group-a"],"mode":"derived","evaluation":{"strategy":"weighted","weights":{"group-a":0.95,"group-b":0.05}}}`)}
	code, data := request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{g, b, a}, RequestID: "groups-mixed"}, "")
	if code != 202 {
		t.Fatalf("mixed group apply %d %s", code, data)
	}
	read := func() resourcegroup.Assessment {
		t.Helper()
		code, data := request("GET", "/api/v1/resource-groups/test-group/assessment", nil, "")
		var out resourcegroup.Assessment
		if code != 200 || json.Unmarshal(data, &out) != nil {
			t.Fatalf("group read %d %s", code, data)
		}
		return out
	}
	if v := read(); v.State == nil || *v.State != "unknown" {
		t.Fatal("missing states became healthy", v)
	}
	ar, _, _ := s.Resource(ctx, "group-a")
	br, _, _ := s.Resource(ctx, "group-b")
	for _, row := range []struct {
		v     resources.View
		state string
	}{{ar, "healthy"}, {br, "down"}} {
		err = s.SaveAssessment(ctx, monitor.Assessment{Monitor: row.v.Manifest.Metadata.Name, Revision: row.v.Revision, At: time.Now(), State: row.state, Zones: []monitor.ZoneAssessment{{Zone: "office", State: row.state, Coverage: "complete", Evidence: []string{row.v.UID}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	if v := read(); *v.State != "degraded" || v.CoverageState != "sufficient" {
		t.Fatal("default weighted hid down", v)
	}
	current, _, _ := s.ResourceKind(ctx, "ResourceGroup", "test-group")
	if code, _ := request("DELETE", "/api/v1/resources/Monitor/group-b", nil, br.ResourceVersion); code != 409 {
		t.Fatal("explicit member deleted", code)
	}
	g.Spec = json.RawMessage(`{"members":["group-a","group-b"],"mode":"derived","evaluation":{"strategy":"weighted","weights":{"group-a":0.95,"group-b":0.05},"healthyMin":0.9}}`)
	if code, _ := request("PUT", "/api/v1/resources/ResourceGroup/test-group", g, ""); code != 428 {
		t.Fatal("missing If-Match accepted", code)
	}
	if code, data := request("PUT", "/api/v1/resources/ResourceGroup/test-group", g, current.ResourceVersion); code != 200 {
		t.Fatalf("group update %d %s", code, data)
	}
	if v := read(); *v.State != "healthy" {
		t.Fatal("explicit threshold ignored", v)
	}
	if err = s.SaveAssessment(ctx, monitor.Assessment{Monitor: "group-a", Revision: "old", At: time.Now(), State: "healthy"}); err != nil {
		t.Fatal(err)
	}
	if v := read(); *v.State != "unknown" || v.CoverageState != "degraded" {
		t.Fatal("old revision leaked", v)
	}
	if code, _ := request("GET", "/api/v1/resource-groups/test-group/assessment?limit=1", nil, ""); code != 400 {
		t.Fatal("unexpected query accepted", code)
	}
	current, _, _ = s.ResourceKind(ctx, "ResourceGroup", "test-group")
	if code, _ := request("DELETE", "/api/v1/resources/ResourceGroup/test-group", nil, current.ResourceVersion); code != 204 {
		t.Fatal("group delete", code)
	}
	for _, v := range []resources.View{ar, br} {
		if code, _ := request("DELETE", "/api/v1/resources/Monitor/"+v.Manifest.Metadata.Name, nil, v.ResourceVersion); code != 204 {
			t.Fatal("member cleanup", code)
		}
	}
}
