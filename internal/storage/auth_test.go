package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/httpapi"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestPostgresBrowserSessionsAndCSRF(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, e := Open(ctx, strings.TrimSpace(string(b)))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	username := "auth-" + newID()
	password := "test-password-unique-long"
	hash, e := auth.HashPassword(password)
	if e != nil {
		t.Fatal(e)
	}
	created, e := s.BootstrapUser(ctx, username, hash)
	if e != nil || !created {
		t.Fatal("isolated initial bootstrap", e)
	}
	if created, e = s.BootstrapUser(ctx, "replacement", hash); e != nil || created {
		t.Fatal("bootstrap overwrote identity", e)
	}
	service := auth.New(s)
	h, e := httpapi.NewWithSessions("service", nil, func(context.Context, string) (monitor.Assessment, bool, error) {
		return monitor.Assessment{Monitor: "service", At: time.Now().UTC(), State: "unknown"}, true, nil
	}, nil, service)
	if e != nil {
		t.Fatal(e)
	}
	h.SetHistoryLoader(s.ReadObservations)
	h.SetDiagnostics(s)
	configureAlertViews(h, s)
	configureResourceTest(t, ctx, s, h)
	server := httptest.NewTLSServer(h)
	defer server.Close()
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	request := func(method, path string, body any, csrf, origin string) (int, []byte, []*http.Cookie) {
		t.Helper()
		var input io.Reader
		if body != nil {
			data, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			input = bytes.NewReader(data)
		}
		r, e := http.NewRequest(method, server.URL+path, input)
		if e != nil {
			t.Fatal(e)
		}
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
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
		return response.StatusCode, data, response.Cookies()
	}
	input := map[string]string{"username": username, "password": password}
	if code, _, _ := request("POST", "/api/v1/auth/login", input, "", "https://foreign.example"); code != 403 {
		t.Fatal("login CSRF accepted", code)
	}
	if code, _, _ := request("POST", "/api/v1/auth/login", map[string]string{"username": username, "password": "wrong"}, "", server.URL); code != 401 {
		t.Fatal("invalid password accepted", code)
	}
	code, data, cookies := request("POST", "/api/v1/auth/login", input, "", server.URL)
	if code != 200 || len(cookies) != 1 {
		t.Fatal("login failed", code)
	}
	cookie := cookies[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatal("unsafe cookie")
	}
	var view auth.SessionView
	if e = json.Unmarshal(data, &view); e != nil || view.Role != "admin" || view.CSRFToken == "" {
		t.Fatal("invalid session view", e)
	}
	if strings.Contains(string(data), password) || strings.Contains(string(data), cookie.Value) {
		t.Fatal("credential disclosed")
	}
	if _, e = auth.New(s).Me(ctx, cookie.Value); e != nil {
		t.Fatal("session not durable across service restart", e)
	}
	if code, _, _ := request("GET", "/api/v1/monitors/service/assessment", nil, "", ""); code != 200 {
		t.Fatal("cookie cannot read assessment", code)
	}
	testAPITokenUseCases(t, ctx, s, server, client, view)
	testResourceUseCases(t, ctx, s, server, client, view)
	testApplyUseCases(t, ctx, s, server, client, view)
	testProfileResources(t, ctx, s, server, client, view)
	testDiagnosticViews(t, ctx, s, server, client)
	testDiagnosticControl(t, ctx, s, server, client)
	testAlertViews(t, ctx, s, server, client)
	testAlertAcknowledgement(t, ctx, s, server, client)
	// Equal database timestamps require ID tie-breaking, including sub-microsecond payload times.
	finished := time.Now().UTC().Truncate(time.Second).Add(123456789 * time.Nanosecond)
	for _, id := range []string{"history-a", "history-b", "history-c"} {
		o := monitor.Observation{ID: id, Monitor: "service", Revision: "historical-revision", Check: "http", Agent: "test-agent", Zone: "west", StartedAt: finished.Add(-time.Second), FinishedAt: finished, Execution: "completed", Outcome: "failure"}
		if id == "history-a" {
			o.Zone = "east"
			o.Execution = "execution_failed"
			o.Outcome = "unknown"
		}
		payload, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.pool.Exec(ctx, `INSERT INTO nmo_observations(id,agent_id,monitor,revision,check_id,finished_at,payload,payload_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, o.ID, o.Agent, o.Monitor, o.Revision, o.Check, o.FinishedAt, payload, []byte{1}); err != nil {
			t.Fatal(err)
		}
	}
	code, data, _ = request("GET", "/api/v1/monitors/service/observations?limit=1", nil, "", "")
	var page httpapi.ObservationPage
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ObservationID != "history-c" || page.NextCursor == nil {
		t.Fatal("history first page", code)
	}
	if page.Items[0].ConfigRevision != "historical-revision" || page.Items[0].ProbeOutcome != "failure" || page.Items[0].ReceivedAt.IsZero() {
		t.Fatal("history provenance lost")
	}
	cursor := *page.NextCursor
	code, data, _ = request("GET", "/api/v1/monitors/service/observations?limit=1&cursor="+url.QueryEscape(cursor), nil, "", "")
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ObservationID != "history-b" || page.NextCursor == nil {
		t.Fatal("history keyset skipped equal timestamp", code)
	}
	code, data, _ = request("GET", "/api/v1/monitors/service/observations?limit=1&cursor="+url.QueryEscape(*page.NextCursor), nil, "", "")
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ObservationID != "history-a" || page.NextCursor != nil {
		t.Fatal("history last page", code)
	}
	for _, query := range []string{"limit=0", "limit=201", "limit=1&limit=2", "since=invalid", "until=2020-01-01T00:00:00Z&since=2021-01-01T00:00:00Z", "zone=east&cursor=" + url.QueryEscape(cursor), "unknown=x", "zone=%zz"} {
		if code, _, _ := request("GET", "/api/v1/monitors/service/observations?"+query, nil, "", ""); code != 400 {
			t.Fatal("invalid history query accepted", query, code)
		}
	}
	code, data, _ = request("GET", "/api/v1/monitors/service/observations?zone=east", nil, "", "")
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ExecutionStatus != "execution_failed" || page.Items[0].ProbeOutcome != "unknown" {
		t.Fatal("zone filter or execution/probe distinction", code)
	}
	interval := url.Values{"since": {finished.Add(-time.Second).Format(time.RFC3339Nano)}, "until": {finished.Add(time.Second).Format(time.RFC3339Nano)}}
	code, data, _ = request("GET", "/api/v1/monitors/service/observations?"+interval.Encode(), nil, "", "")
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 3 {
		t.Fatal("history interval filtering", code)
	}
	code, data, _ = request("GET", "/api/v1/monitors/service/observations?since="+url.QueryEscape(finished.Add(time.Second).Format(time.RFC3339Nano)), nil, "", "")
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Fatal("empty history must be a valid page", code)
	}
	// Password change must be atomic across all sessions and preserve CSRF protection.
	_, otherSession, e := service.Login(ctx, username, password)
	if e != nil {
		t.Fatal(e)
	}
	change := map[string]string{"currentPassword": password, "newPassword": "replacement-password-long"}
	if code, _, _ := request("POST", "/api/v1/auth/change-password", change, "", server.URL); code != 403 {
		t.Fatal("password change without CSRF", code)
	}
	if code, _, _ := request("POST", "/api/v1/auth/change-password", map[string]string{"currentPassword": "wrong", "newPassword": change["newPassword"]}, view.CSRFToken, server.URL); code != 401 {
		t.Fatal("wrong current password accepted", code)
	}
	if code, _, _ := request("POST", "/api/v1/auth/change-password", map[string]string{"currentPassword": password, "newPassword": "short"}, view.CSRFToken, server.URL); code != 422 {
		t.Fatal("weak password accepted", code)
	}
	if _, e = service.Me(ctx, otherSession); e != nil {
		t.Fatal("rejected change revoked session", e)
	}
	oldCookie := cookie
	code, data, cookies = request("POST", "/api/v1/auth/change-password", change, view.CSRFToken, server.URL)
	if code != 200 || len(cookies) != 1 {
		t.Fatal("password change failed", code)
	}
	if _, e = service.Me(ctx, oldCookie.Value); e != auth.ErrCredentials {
		t.Fatal("old session survived password change", e)
	}
	if _, e = service.Me(ctx, otherSession); e != auth.ErrCredentials {
		t.Fatal("parallel session survived password change", e)
	}
	oldCSRF := view.CSRFToken
	cookie = cookies[0]
	if e = json.Unmarshal(data, &view); e != nil || view.CSRFToken == oldCSRF || cookie.Value == oldCookie.Value {
		t.Fatal("credentials not rotated", e)
	}
	if _, e = auth.New(s).Me(ctx, cookie.Value); e != nil {
		t.Fatal("replacement session not durable", e)
	}
	if _, _, e = service.Login(ctx, username, password); e != auth.ErrCredentials {
		t.Fatal("old password still usable", e)
	}
	password = change["newPassword"]
	if _, _, e = service.Login(ctx, username, password); e != nil {
		t.Fatal("new password unusable", e)
	}
	if code, _, _ := request("POST", "/api/v1/auth/logout", nil, "", server.URL); code != 403 {
		t.Fatal("logout without CSRF", code)
	}
	if code, _, _ := request("GET", "/api/v1/auth/me", nil, "", ""); code != 200 {
		t.Fatal("rejected CSRF revoked session", code)
	}
	if code, _, _ := request("POST", "/api/v1/auth/logout", nil, view.CSRFToken, server.URL); code != 204 {
		t.Fatal("logout failed", code)
	}
	if _, e = service.Me(ctx, cookie.Value); e != auth.ErrCredentials {
		t.Fatal("revoked cookie reusable", e)
	}
	// Failed-login state must survive a fresh Service instance.
	for i := 0; i < 5; i++ {
		_, _, e = service.Login(ctx, username, "wrong")
		if e != auth.ErrCredentials {
			t.Fatal("failure not recorded", e)
		}
	}
	if _, _, e = auth.New(s).Login(ctx, username, password); e != auth.ErrCredentials {
		t.Fatal("restart bypassed lockout", e)
	}
	if _, e = s.pool.Exec(ctx, "UPDATE nmo_users SET locked_until=NULL,failed_logins=0,role='viewer' WHERE username=$1", username); e != nil {
		t.Fatal(e)
	}
	v, raw, e := auth.New(s).Login(ctx, username, password)
	if e != nil || v.Role != "viewer" {
		t.Fatal("viewer login", e)
	}
	for _, scope := range v.Scopes {
		if scope == "resources:write" || scope == "system:admin" {
			t.Fatal("viewer escalated")
		}
	}
	if _, e = s.pool.Exec(ctx, "UPDATE nmo_browser_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", auth.HashToken(raw)); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Me(ctx, raw); e != auth.ErrCredentials {
		t.Fatal("expired session accepted", e)
	}
	testUserManagement(t, ctx, s, server, username, password)
}
