package storage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
)

func testDiagnosticViews(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client) {
	t.Helper()
	request := func(method, path, bearer string) (int, []byte) {
		t.Helper()
		r, err := http.NewRequest(method, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		response, err := client.Do(r)
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
	// Deliberately equal DB microsecond timestamps: pagination must use the ID.
	started := time.Now().UTC().Truncate(time.Microsecond)
	for _, id := range []string{"diag-view-a", "diag-view-b", "diag-view-c"} {
		_, err := s.pool.Exec(ctx, `INSERT INTO nmo_diagnostic_runs(id,episode_id,monitor,revision,profile_hash,profile_ref,mode,max_checks,used_checks,deadline,state,started_at) VALUES($1,'episode','diagnostic-service','old-revision','hash','profile','adaptive',6,2,$2,'running',$3)`, id, started.Add(time.Minute), started)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.pool.Exec(ctx, `UPDATE nmo_diagnostic_runs SET state='inconclusive',reason='controller_restart',finished_at=now(),profile_ref='',mode='static' WHERE id='diag-view-a';
 UPDATE nmo_diagnostic_runs SET state='completed',reason='unique',finished_at=now(),report='{"hypotheses":["tcp-failure"],"status":"completed"}' WHERE id='diag-view-b'`)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"task-view-a", "task-view-b"} {
		if _, err = s.pool.Exec(ctx, `INSERT INTO nmo_diagnostic_tasks(id,run_id,agent_id,monitor,revision,zone,check_id,deadline,check_spec,state,attempt_no) VALUES($1,'diag-view-b','branch','diagnostic-service','old-revision','office','dns-action',$2,'{"secret":"must-not-expose"}','reserved',2)`, id, started.Add(time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO nmo_observations(id,agent_id,monitor,revision,check_id,finished_at,payload,payload_hash,diagnostic_task_id) VALUES('task-view-a','branch','diagnostic-service','old-revision','dns-action',$1,'{"body":"must-not-expose"}','x','task-view-a')`, started); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE nmo_diagnostic_tasks SET state='finished' WHERE id='task-view-a'`); err != nil {
		t.Fatal(err)
	}
	code, data := request("GET", "/api/v1/diagnostics?monitor=diagnostic-service&limit=1", "")
	var page diagnosticview.Sessions
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ID != "diag-view-c" || page.NextCursor == nil {
		t.Fatalf("diagnostics first page %d %s", code, data)
	}
	cursor := url.QueryEscape(*page.NextCursor)
	code, data = request("GET", "/api/v1/diagnostics?monitor=diagnostic-service&limit=1&cursor="+cursor, "")
	if code != 200 || json.Unmarshal(data, &page) != nil || page.Items[0].ID != "diag-view-b" || page.Items[0].BudgetUsed.AdditionalChecks != 2 || len(page.Items[0].EvidenceIDs) != 1 || page.Items[0].RemainingHypotheses == nil {
		t.Fatal("diagnostics stable cursor/budget", code)
	}
	if code, _ = request("GET", "/api/v1/diagnostics?monitor=other&cursor="+cursor, ""); code != 400 {
		t.Fatal("cross-filter cursor accepted", code)
	}
	var view diagnosticview.Session
	code, data = request("GET", "/api/v1/diagnostics/diag-view-a", "")
	if code != 200 || json.Unmarshal(data, &view) != nil || view.Mode != "static" || view.ProfileRef != "" || view.RemainingHypotheses != nil || view.StopReason == nil || *view.StopReason != "controller_restart" {
		t.Fatal("crash evidence invented", code)
	}
	var tasks diagnosticview.Tasks
	code, data = request("GET", "/api/v1/diagnostics/diag-view-b/tasks?limit=1", "")
	if code != 200 || json.Unmarshal(data, &tasks) != nil || len(tasks.Items) != 1 || tasks.Items[0].ID != "task-view-a" || tasks.Items[0].AttemptNo != 2 || len(tasks.Items[0].ObservationIDs) != 1 || tasks.NextCursor == nil || strings.Contains(string(data), "must-not-expose") {
		t.Fatal("safe physical task view", code)
	}
	taskCursor := url.QueryEscape(*tasks.NextCursor)
	code, data = request("GET", "/api/v1/diagnostics/diag-view-b/tasks?limit=1&cursor="+taskCursor, "")
	if code != 200 || json.Unmarshal(data, &tasks) != nil || tasks.Items[0].ID != "task-view-b" || len(tasks.Items[0].ObservationIDs) != 0 {
		t.Fatal("missing result became evidence", code)
	}
	if code, _ = request("GET", "/api/v1/diagnostics/diag-view-a/tasks?cursor="+taskCursor, ""); code != 400 {
		t.Fatal("cross-session cursor", code)
	}
	for _, path := range []string{"/api/v1/diagnostics?limit=201", "/api/v1/diagnostics?state=healthy", "/api/v1/diagnostics?state=running&state=completed", "/api/v1/diagnostics?since=bad"} {
		if code, _ = request("GET", path, ""); code != 400 {
			t.Fatal("invalid diagnostic query", code)
		}
	}
	if code, _ = request("GET", "/api/v1/diagnostics/missing/tasks", ""); code != 404 {
		t.Fatal("missing session tasks", code)
	}
	if code, _ = request("POST", "/api/v1/diagnostics/diag-view-b/cancel", ""); code != 403 {
		t.Fatal("mutation without CSRF accepted", code)
	}
	var raw string
	parsed, _ := url.Parse(server.URL)
	for _, cookie := range client.Jar.Cookies(parsed) {
		if cookie.Name == "netmonorc_session" {
			raw = cookie.Value
		}
	}
	token, err := auth.New(s).IssueToken(ctx, raw, auth.APITokenCreate{Name: "no-diagnostics", Scopes: []string{"resources:read"}})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("GET", "/api/v1/diagnostics", token.Token); code != 403 {
		t.Fatal("diagnostics scope bypassed", code)
	}
}
