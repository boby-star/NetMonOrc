package storage

import (
	"bytes"
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
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func testAlertAcknowledgement(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client) {
	t.Helper()
	var cookie string
	parsed, _ := url.Parse(server.URL)
	for _, c := range client.Jar.Cookies(parsed) {
		if c.Name == "netmonorc_session" {
			cookie = c.Value
		}
	}
	identity, err := auth.New(s).Me(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	bearer := ""
	request := func(method, path string, body any, tag, csrf string) (int, []byte, string) {
		t.Helper()
		var input io.Reader
		if body != nil {
			data, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			input = bytes.NewReader(data)
		}
		req, e := http.NewRequest(method, server.URL+path, input)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Origin", server.URL)
		req.Header.Set("Content-Type", "application/json")
		if tag != "" {
			req.Header.Set("If-Match", tag)
		}
		if csrf != "" {
			req.Header.Set("X-CSRF-Token", csrf)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		response, e := client.Do(req)
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
	rule, err := alerting.New(alerting.Rule{Name: "ack-rule", Monitor: "ack-monitor", Channel: "read-webhook", Scope: "monitor", Severity: "critical", When: `monitor.state == "down"`, ResolveWhen: `monitor.state == "healthy"`, Enabled: true, NotifyEvents: []string{"firing", "resolved"}})
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	a := monitor.Assessment{Monitor: rule.Rule.Monitor, Revision: "ack-revision", At: at, State: "down", Zones: []monitor.ZoneAssessment{{Zone: "office", Coverage: "complete", Reason: "http_503", Evidence: []string{"ack-proof"}}}}
	if err = s.SaveAssessment(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err = s.EvaluateAlert(ctx, rule, alertViewEndpoint(), a); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Alerts(ctx, alertview.Query{Monitor: a.Monitor, Limit: 10})
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	id := rows[0].ID
	path := "/api/v1/alerts/" + id
	code, _, etag := request("GET", path, nil, "", "")
	if code != 200 || etag == "" {
		t.Fatal("alert ETag absent", code)
	}
	note := strings.Repeat("😀", 1024)
	body := alertview.Acknowledge{Note: note}
	if code, _, _ = request("POST", path+"/acknowledge", body, etag, ""); code != 403 {
		t.Fatal("missing CSRF accepted", code)
	}
	if code, _, _ = request("POST", path+"/acknowledge", body, "", identity.CSRFToken); code != 428 {
		t.Fatal("missing precondition accepted", code)
	}
	if code, _, _ = request("POST", path+"/acknowledge", alertview.Acknowledge{Note: strings.Repeat("x", 1025)}, etag, identity.CSRFToken); code != 422 {
		t.Fatal("oversize note accepted", code)
	}
	if code, _, _ = request("POST", path+"/acknowledge", alertview.Acknowledge{Note: "bad\x00note"}, etag, identity.CSRFToken); code != 422 {
		t.Fatal("NUL note accepted", code)
	}
	var before []byte
	var transitions, outbox int
	if err = s.pool.QueryRow(ctx, `SELECT payload,(SELECT count(*) FROM nmo_alert_events),(SELECT count(*) FROM nmo_webhook_outbox) FROM nmo_alerts WHERE id=$1`, id).Scan(&before, &transitions, &outbox); err != nil {
		t.Fatal(err)
	}
	code, data, ackTag := request("POST", path+"/acknowledge", body, etag, identity.CSRFToken)
	var acknowledged alertview.Alert
	if code != 200 || json.Unmarshal(data, &acknowledged) != nil || acknowledged.AcknowledgedAt == nil || acknowledged.State != "firing" || ackTag == etag {
		t.Fatal("acknowledgement missing", code)
	}
	if strings.Contains(string(data), "😀") {
		t.Fatal("operator note exposed in Alert DTO")
	}
	var unchanged bool
	if err = s.pool.QueryRow(ctx, `SELECT payload=$2::jsonb AND (SELECT count(*) FROM nmo_alert_events)=$3 AND (SELECT count(*) FROM nmo_webhook_outbox)=$4 FROM nmo_alerts WHERE id=$1`, id, before, transitions, outbox).Scan(&unchanged); err != nil || !unchanged {
		t.Fatal("ack mutated FSM or delivery", err)
	}
	var assessmentState string
	if err = s.pool.QueryRow(ctx, `SELECT payload->>'state' FROM nmo_assessments WHERE monitor=$1`, a.Monitor).Scan(&assessmentState); err != nil || assessmentState != "down" {
		t.Fatal("ack resolved service", err)
	}
	if code, _, _ = request("POST", path+"/acknowledge", body, etag, identity.CSRFToken); code != 412 {
		t.Fatal("stale acknowledgement accepted", code)
	}
	code, data, repeatedTag := request("POST", path+"/acknowledge", body, ackTag, identity.CSRFToken)
	var repeated alertview.Alert
	if code != 200 || json.Unmarshal(data, &repeated) != nil || !repeated.AcknowledgedAt.Equal(*acknowledged.AcknowledgedAt) || repeatedTag != ackTag {
		t.Fatal("ack retry overwrote first receipt", code)
	}
	if code, _, _ = request("POST", path+"/acknowledge", alertview.Acknowledge{Note: "replacement"}, ackTag, identity.CSRFToken); code != 409 {
		t.Fatal("note overwritten", code)
	}
	a.At = at.Add(time.Second)
	a.State = "unknown"
	if _, err = s.EvaluateAlert(ctx, rule, alertViewEndpoint(), a); err != nil {
		t.Fatal(err)
	}
	held, _, err := s.Alert(ctx, id)
	if err != nil || held.AcknowledgedAt == nil {
		t.Fatal("unknown erased acknowledgement", err)
	}
	a.At = at.Add(2 * time.Second)
	a.State = "healthy"
	if _, err = s.EvaluateAlert(ctx, rule, alertViewEndpoint(), a); err != nil {
		t.Fatal(err)
	}
	_, _, resolvedTag := request("GET", path, nil, "", "")
	if code, _, _ = request("POST", path+"/acknowledge", body, resolvedTag, identity.CSRFToken); code != 409 {
		t.Fatal("inactive alert acknowledged", code)
	}
	a.At = at.Add(3 * time.Second)
	a.State = "down"
	if _, err = s.EvaluateAlert(ctx, rule, alertViewEndpoint(), a); err != nil {
		t.Fatal(err)
	}
	current, _, err := s.Alert(ctx, id)
	if err != nil || current.AcknowledgedAt != nil {
		t.Fatal("ack carried to next firing", err)
	}
	if code, _, _ = request("POST", path+"/acknowledge", body, ackTag, identity.CSRFToken); code != 412 {
		t.Fatal("old tab acknowledged new firing", code)
	}
	_, _, currentTag := request("GET", path, nil, "", "")
	reader, err := auth.New(s).IssueToken(ctx, cookie, auth.APITokenCreate{Name: "read-only-ack", Scopes: []string{"alerts:read"}})
	if err != nil {
		t.Fatal(err)
	}
	bearer = reader.Token
	if code, _, _ = request("POST", path+"/acknowledge", body, currentTag, ""); code != 403 {
		t.Fatal("read token acknowledged alert", code)
	}
	bearer = ""
	if code, _, _ = request("POST", path+"/acknowledge", alertview.Acknowledge{Note: "second cycle"}, currentTag, identity.CSRFToken); code != 200 {
		t.Fatal("next cycle ack failed", code)
	}
	var count int
	if err = s.pool.QueryRow(ctx, `SELECT count(*) FROM nmo_alert_acknowledgements WHERE alert_id=$1`, id).Scan(&count); err != nil || count != 2 {
		t.Fatal("per-cycle receipt history lost", err)
	}
}
