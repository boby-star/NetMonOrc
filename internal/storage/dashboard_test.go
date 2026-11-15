package storage

import (
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/dashboard"
	"github.com/boby-star/NetMonOrc/internal/httpapi"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func configureDashboardTest(h *httpapi.Handler, s *Store) {
	h.SetDashboard(func(ctx context.Context, reads dashboard.Reads) (dashboard.Source, error) {
		return s.Dashboard(ctx, "service", "dashboard-runtime", reads)
	})
	h.SetAgents(func() []agentstatus.View {
		return []agentstatus.View{{OnlineState: "online"}, {OnlineState: "offline"}}
	})
}
func testDashboard(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client) {
	t.Helper()
	request := func(path, bearer string) (int, []byte) {
		t.Helper()
		req, e := http.NewRequest("GET", server.URL+path, nil)
		if e != nil {
			t.Fatal(e)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		res, e := client.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		data, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, data
	}
	if _, err := s.pool.Exec(ctx, `UPDATE nmo_resources SET effective_revision='dashboard-runtime' WHERE kind='Monitor' AND name='service'`); err != nil {
		t.Fatal(err)
	}
	a := monitor.Assessment{Monitor: "service", Revision: "dashboard-runtime", At: time.Now(), State: "healthy", Zones: []monitor.ZoneAssessment{{Coverage: "complete", Evidence: []string{"dashboard-proof"}}}}
	if err := s.SaveAssessment(ctx, a); err != nil {
		t.Fatal(err)
	}
	code, data := request("/api/v1/dashboard/summary", "")
	var v dashboard.Summary
	if code != 200 || json.Unmarshal(data, &v) != nil || v.MonitorStates["healthy"] != 1 || v.CoverageStates["sufficient"] != 1 || v.AgentCounts["online"] != 1 || v.AgentCounts["offline"] != 1 || v.FiringAlerts == nil || v.RecentDiagnostics == nil {
		t.Fatal("dashboard facts missing", code)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM nmo_resources WHERE kind='Monitor'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if v.MonitorStates["unknown"] != count-1 {
		t.Fatal("unselected Monitor treated as healthy")
	}
	a.At = time.Now().Add(-10 * time.Second)
	if err := s.SaveAssessment(ctx, a); err != nil {
		t.Fatal(err)
	}
	code, data = request("/api/v1/dashboard/summary", "")
	if code != 200 || json.Unmarshal(data, &v) != nil || v.MonitorStates["healthy"] != 0 || v.MonitorStates["unknown"] != count || v.AgentCounts["online"] != 1 {
		t.Fatal("Agent online promoted stale resource", code)
	}
	a.At = time.Now()
	if err := s.SaveAssessment(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE nmo_resources SET effective_revision='rollout' WHERE kind='Monitor' AND name='service'`); err != nil {
		t.Fatal(err)
	}
	code, data = request("/api/v1/dashboard/summary", "")
	if code != 200 || json.Unmarshal(data, &v) != nil || v.MonitorStates["healthy"] != 0 {
		t.Fatal("rollout reused old healthy", code)
	}
	var cookie string
	parsed, _ := url.Parse(server.URL)
	for _, c := range client.Jar.Cookies(parsed) {
		if c.Name == "netmonorc_session" {
			cookie = c.Value
		}
	}
	token, err := auth.New(s).IssueToken(ctx, cookie, auth.APITokenCreate{Name: "dashboard-resources", Scopes: []string{"resources:read"}})
	if err != nil {
		t.Fatal(err)
	}
	code, data = request("/api/v1/dashboard/summary", token.Token)
	var limited map[string]json.RawMessage
	if code != 200 || json.Unmarshal(data, &limited) != nil {
		t.Fatal(code)
	}
	for _, key := range []string{"agentCounts", "firingAlerts", "recentDiagnostics"} {
		if _, ok := limited[key]; ok {
			t.Fatal("dashboard leaked scope", key)
		}
	}
	if code, _ := request("/api/v1/dashboard/summary?limit=1", ""); code != 400 {
		t.Fatal("unexpected dashboard query", code)
	}
}
