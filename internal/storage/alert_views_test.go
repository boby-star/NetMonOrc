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

	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/alertview"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/httpapi"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/webhook"
)

func alertViewEndpoint() webhook.Endpoint {
	return webhook.Endpoint{Name: "read-webhook", URL: "https://192.0.2.5", Enabled: true, Timeout: time.Second, MaxAttempts: 2}
}
func configureAlertViews(h *httpapi.Handler, s *Store) { h.SetAlerts(s, alertViewEndpoint().Hash()) }
func testAlertViews(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client) {
	t.Helper()
	request := func(method, path, bearer string) (int, []byte) {
		t.Helper()
		r, e := http.NewRequest(method, server.URL+path, nil)
		if e != nil {
			t.Fatal(e)
		}
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
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
		return response.StatusCode, data
	}
	rule, err := alerting.New(alerting.Rule{Name: "read-alert", Monitor: "alert-monitor", Channel: "read-webhook", Scope: "monitor", Severity: "critical", When: `monitor.state == "down"`, ResolveWhen: `monitor.state == "healthy"`, Enabled: true, Cooldown: time.Minute, NotifyEvents: []string{"firing", "resolved"}})
	if err != nil {
		t.Fatal(err)
	}
	endpoint := alertViewEndpoint()
	at := time.Now().UTC()
	a := monitor.Assessment{Monitor: rule.Rule.Monitor, Revision: "alert-revision", At: at, State: "down", Zones: []monitor.ZoneAssessment{{Zone: "office", Coverage: "complete", Reason: "http_503", Evidence: []string{"proof-down"}}}}
	if _, err = s.EvaluateAlert(ctx, rule, endpoint, a); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Alerts(ctx, alertview.Query{Monitor: a.Monitor, Limit: 20})
	if err != nil || len(rows) != 1 {
		t.Fatal("alert projection missing", err)
	}
	first := rows[0]
	if first.State != "firing" || first.Severity != "critical" || first.FiredAt == nil || len(first.EvidenceIDs) != 1 {
		t.Fatal("firing view missing facts")
	}
	job, err := s.ClaimWebhook(ctx, endpoint)
	if err != nil || job == nil {
		t.Fatal("notification not queued", err)
	}
	if err = s.FinishWebhook(ctx, *job, endpoint, 503); err != nil {
		t.Fatal(err)
	}
	a.At = at.Add(time.Second)
	a.State = "unknown"
	a.Zones[0].Evidence = []string{"uncertain-proof"}
	a.Zones[0].Reason = "missing_coverage"
	if _, err = s.EvaluateAlert(ctx, rule, endpoint, a); err != nil {
		t.Fatal(err)
	}
	held, _, err := s.Alert(ctx, first.ID)
	if err != nil || held.State != "firing" || held.EvidenceIDs[0] != "proof-down" || !held.FiredAt.Equal(*first.FiredAt) {
		t.Fatal("unknown changed triggering evidence", err)
	}
	stale := a
	stale.At = at.Add(-time.Second)
	stale.State = "healthy"
	if _, err = s.EvaluateAlert(ctx, rule, endpoint, stale); err != nil {
		t.Fatal(err)
	}
	held, _, err = s.Alert(ctx, first.ID)
	if err != nil || held.State != "firing" || held.EvidenceIDs[0] != "proof-down" {
		t.Fatal("old assessment rewrote view", err)
	}
	code, data := request("GET", "/api/v1/webhook/deliveries?state=pending", "")
	var deliveries alertview.DeliveryPage
	if code != 200 || json.Unmarshal(data, &deliveries) != nil || len(deliveries.Items) != 1 || deliveries.Items[0].Attempts != 1 || deliveries.Items[0].LastHTTPStatus == nil || *deliveries.Items[0].LastHTTPStatus != 503 || deliveries.Items[0].LastAttemptAt == nil {
		t.Fatal("delivery attempts view", code)
	}
	for _, secret := range []string{"https://", "payload", "lease_token", "secret"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("private delivery data exposed")
		}
	}
	a.At = at.Add(2 * time.Second)
	a.State = "healthy"
	a.Zones[0].Evidence = []string{"proof-recovered"}
	if _, err = s.EvaluateAlert(ctx, rule, endpoint, a); err != nil {
		t.Fatal(err)
	}
	resolved, _, err := s.Alert(ctx, first.ID)
	if err != nil || resolved.State != "resolved" || resolved.ResolvedAt == nil {
		t.Fatal("resolve not projected", err)
	}
	a.At = at.Add(3 * time.Second)
	a.State = "down"
	a.Zones[0].Evidence = []string{"proof-second"}
	if _, err = s.EvaluateAlert(ctx, rule, endpoint, a); err != nil {
		t.Fatal(err)
	}
	newCycle, _, err := s.Alert(ctx, first.ID)
	if err != nil || newCycle.ID != first.ID || newCycle.ResolvedAt != nil || !newCycle.FiredAt.Equal(a.At) {
		t.Fatal("cycle timestamps not reset", err)
	}
	code, data = request("GET", "/api/v1/alerts/"+first.ID+"/history?limit=1", "")
	var history alertview.HistoryPage
	if code != 200 || json.Unmarshal(data, &history) != nil || len(history.Items) != 1 || !history.Items[0].Suppressed || history.NextCursor == nil {
		t.Fatal("suppressed transition omitted", code)
	}
	cursor := url.QueryEscape(*history.NextCursor)
	firstEvent := history.Items[0].ID
	code, data = request("GET", "/api/v1/alerts/"+first.ID+"/history?limit=1&cursor="+cursor, "")
	if code != 200 || json.Unmarshal(data, &history) != nil || history.Items[0].ID == firstEvent || history.Items[0].To != "resolved" {
		t.Fatal("history cursor duplicated event", code)
	}
	secondRule := rule.Rule
	secondRule.Name = "read-other"
	secondRule.Severity = "warning"
	second, err := alerting.New(secondRule)
	if err != nil {
		t.Fatal(err)
	}
	otherEndpoint := endpoint
	otherEndpoint.URL = "https://192.0.2.6"
	if _, err = s.EvaluateAlert(ctx, second, otherEndpoint, a); err != nil {
		t.Fatal(err)
	}
	code, data = request("GET", "/api/v1/alerts?monitor=alert-monitor&limit=1", "")
	var page alertview.AlertPage
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.NextCursor == nil {
		t.Fatal("alert page", code)
	}
	firstID := page.Items[0].ID
	alertCursor := url.QueryEscape(*page.NextCursor)
	code, data = request("GET", "/api/v1/alerts?monitor=alert-monitor&limit=1&cursor="+alertCursor, "")
	if code != 200 || json.Unmarshal(data, &page) != nil || page.Items[0].ID == firstID {
		t.Fatal("alert cursor duplicates", code)
	}
	if code, _ = request("GET", "/api/v1/alerts?severity=warning&cursor="+alertCursor, ""); code != 400 {
		t.Fatal("cross-filter alert cursor", code)
	}
	code, data = request("GET", "/api/v1/webhook/deliveries", "")
	if code != 200 || json.Unmarshal(data, &deliveries) != nil || len(deliveries.Items) != 2 {
		t.Fatal("wrong endpoint history included", code)
	}
	for _, path := range []string{"/api/v1/alerts?state=healthy", "/api/v1/alerts?severity=secret", "/api/v1/webhook/deliveries?limit=201", "/api/v1/alerts?state=firing&state=resolved"} {
		if code, _ = request("GET", path, ""); code != 400 {
			t.Fatal("invalid alert filters", code)
		}
	}
	if code, _ = request("GET", "/api/v1/alerts/missing/history", ""); code != 404 {
		t.Fatal("missing alert history", code)
	}
	// Legacy firing facts must not be reconstructed from a new observation.
	legacyRule := rule.Rule
	legacyRule.Name = "legacy-read"
	legacy, err := alerting.New(legacyRule)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := json.Marshal(alerting.State{Status: "firing", LastEvaluatedAt: at.Add(-time.Second)})
	if _, err = s.pool.Exec(ctx, `INSERT INTO nmo_alerts(logical_key,payload) VALUES('legacy-read/alert-monitor/monitor',$1)`, state); err != nil {
		t.Fatal(err)
	}
	a.State = "unknown"
	a.At = at.Add(4 * time.Second)
	if _, err = s.EvaluateAlert(ctx, legacy, endpoint, a); err != nil {
		t.Fatal(err)
	}
	legacyViews, err := s.Alerts(ctx, alertview.Query{Monitor: a.Monitor, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range legacyViews {
		if v.RuleRef == "legacy-read" && (v.FiredAt != nil || v.StartedAt != nil || len(v.EvidenceIDs) != 0) {
			t.Fatal("legacy cause fabricated")
		}
	}
	var raw string
	parsed, _ := url.Parse(server.URL)
	for _, cookie := range client.Jar.Cookies(parsed) {
		if cookie.Name == "netmonorc_session" {
			raw = cookie.Value
		}
	}
	token, err := auth.New(s).IssueToken(ctx, raw, auth.APITokenCreate{Name: "no-alert-read", Scopes: []string{"resources:read"}})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("GET", "/api/v1/alerts", token.Token); code != 403 {
		t.Fatal("alerts read scope bypass", code)
	}
	if code, _ = request("GET", "/api/v1/webhook/deliveries", token.Token); code != 403 {
		t.Fatal("delivery scope bypass", code)
	}
}
