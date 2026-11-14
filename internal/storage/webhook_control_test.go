package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/operationview"
	"github.com/boby-star/NetMonOrc/internal/webhook"
)

func testWebhookControl(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client) {
	t.Helper()
	var cookie string
	parsed, _ := url.Parse(server.URL)
	for _, c := range client.Jar.Cookies(parsed) {
		if c.Name == "netmonorc_session" {
			cookie = c.Value
		}
	}
	session, err := auth.New(s).Me(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise upgrade from the previous alert-owned outbox foreign key.
	var queueFingerprint string
	if err = s.pool.QueryRow(ctx, `SELECT md5(COALESCE(string_agg(event_id||payload::text,'|' ORDER BY event_id),'')) FROM nmo_webhook_outbox`).Scan(&queueFingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, `ALTER TABLE nmo_webhook_outbox DROP CONSTRAINT nmo_webhook_notification_event_fkey; DELETE FROM nmo_notification_events; ALTER TABLE nmo_webhook_outbox ADD CONSTRAINT nmo_webhook_outbox_event_id_fkey FOREIGN KEY(event_id) REFERENCES nmo_alert_events(id)`); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, s.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal("legacy outbox migration", err)
	}
	reopened.Close()
	var migrated string
	if err = s.pool.QueryRow(ctx, `SELECT md5(COALESCE(string_agg(event_id||payload::text,'|' ORDER BY event_id),'')) FROM nmo_webhook_outbox`).Scan(&migrated); err != nil || migrated != queueFingerprint {
		t.Fatal("migration changed queued event IDs or payload", err)
	}
	bearer := ""
	request := func(method, path string, body any, csrf string) (int, []byte, string) {
		t.Helper()
		var input io.Reader
		if body != nil {
			raw, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			input = bytes.NewReader(raw)
		}
		req, e := http.NewRequest(method, server.URL+path, input)
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", server.URL)
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
		raw, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, raw, response.Header.Get("Location")
	}
	if code, _, _ := request("POST", "/api/v1/webhook/test", struct{}{}, ""); code != 403 {
		t.Fatal("test without CSRF", code)
	}
	if code, _, _ := request("POST", "/api/v1/webhook/test", map[string]string{"message": "forbidden"}, session.CSRFToken); code != 400 {
		t.Fatal("removed message accepted", code)
	}
	if code, _, _ := request("POST", "/api/v1/webhook/test", map[string]string{"url": "https://169.254.169.254/"}, session.CSRFToken); code != 400 {
		t.Fatal("target override accepted", code)
	}
	var alertsBefore, eventsBefore int
	if err = s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM nmo_alerts),(SELECT count(*) FROM nmo_alert_events)`).Scan(&alertsBefore, &eventsBefore); err != nil {
		t.Fatal(err)
	}
	code, data, location := request("POST", "/api/v1/webhook/test", struct{}{}, session.CSRFToken)
	var op operationview.Operation
	var result webhook.TestResult
	if code != 202 || json.Unmarshal(data, &op) != nil || op.Kind != "webhook.test" || op.State != "accepted" || location != "/api/v1/operations/"+op.ID || json.Unmarshal(op.Result, &result) != nil {
		t.Fatal("test queue response", code)
	}
	if code, _, _ := request("POST", "/api/v1/webhook/test", struct{}{}, session.CSRFToken); code != 409 {
		t.Fatal("parallel test admitted", code)
	}
	var raw []byte
	var unchanged bool
	if err = s.pool.QueryRow(ctx, `SELECT payload,(SELECT count(*) FROM nmo_alerts)=$2 AND (SELECT count(*) FROM nmo_alert_events)=$3 FROM nmo_webhook_outbox WHERE event_id=$1`, result.EventID, alertsBefore, eventsBefore).Scan(&raw, &unchanged); err != nil || !unchanged {
		t.Fatal("test created fake alert", err)
	}
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil || len(payload) != 4 || payload["event_type"] != "webhook.test" || payload["schema_version"] != "1" || payload["event_id"] != result.EventID {
		t.Fatal("wrong test payload")
	}
	if _, err = time.Parse(time.RFC3339Nano, payload["occurred_at"].(string)); err != nil {
		t.Fatal(err)
	}
	// Other alerts can be pending too; claim until this bounded test event is reached.
	var job *Delivery
	for i := 0; i < 20; i++ {
		job, err = s.ClaimWebhook(ctx, alertViewEndpoint())
		if err != nil || job == nil {
			t.Fatal("test not claimable", err)
		}
		if job.ID == result.EventID {
			break
		}
		if err = s.FinishWebhook(ctx, *job, alertViewEndpoint(), 200); err != nil {
			t.Fatal(err)
		}
	}
	if job.ID != result.EventID {
		t.Fatal("test event starved")
	}
	running, _, err := s.ReadOperation(ctx, op.ID)
	if err != nil || running.State != "running" {
		t.Fatal("test operation not running", err)
	}
	if err = s.FinishWebhook(ctx, *job, alertViewEndpoint(), 503); err != nil {
		t.Fatal(err)
	}
	retrying, _, err := s.ReadOperation(ctx, op.ID)
	if err != nil || retrying.State != "running" || retrying.FinishedAt != nil {
		t.Fatal("retry became terminal", err)
	}
	job, err = s.ClaimWebhook(ctx, alertViewEndpoint())
	if err != nil || job == nil || job.ID != result.EventID {
		t.Fatal("same event not retried", err)
	}
	if err = s.FinishWebhook(ctx, *job, alertViewEndpoint(), 200); err != nil {
		t.Fatal(err)
	}
	code, data, _ = request("GET", location, nil, "")
	if code != 200 || json.Unmarshal(data, &op) != nil || op.State != "completed" || op.FinishedAt == nil || json.Unmarshal(op.Result, &result) != nil || result.Attempts != 2 || result.LastHTTPStatus == nil || *result.LastHTTPStatus != 200 {
		t.Fatal("delivery not reflected in operation", code)
	}
	reader, err := auth.New(s).IssueToken(ctx, cookie, auth.APITokenCreate{Name: "webhook-operation-reader", Scopes: []string{"alerts:read"}})
	if err != nil {
		t.Fatal(err)
	}
	bearer = reader.Token
	if code, _, _ := request("GET", location, nil, ""); code != 200 {
		t.Fatal("alert scope cannot read test operation", code)
	}
	if code, _, _ := request("POST", "/api/v1/webhook/test", struct{}{}, ""); code != 403 {
		t.Fatal("read token created test", code)
	}
	resourceReader, err := auth.New(s).IssueToken(ctx, cookie, auth.APITokenCreate{Name: "config-operation-reader", Scopes: []string{"resources:read"}})
	if err != nil {
		t.Fatal(err)
	}
	bearer = resourceReader.Token
	if code, _, _ := request("GET", location, nil, ""); code != 403 {
		t.Fatal("resource scope leaked webhook operation", code)
	}
	code, data, _ = request("GET", "/api/v1/operations?limit=200", nil, "")
	var page operationview.Page
	if code != 200 || json.Unmarshal(data, &page) != nil {
		t.Fatal("scoped operation page", code)
	}
	for _, item := range page.Items {
		if item.Kind != "config.apply" {
			t.Fatal("unauthorized kind in page")
		}
	}
	bearer = ""
	code, data, _ = request("POST", "/api/v1/webhook/test", struct{}{}, session.CSRFToken)
	if code != 202 || json.Unmarshal(data, &op) != nil {
		t.Fatal("next test refused", code)
	}
	changed := alertViewEndpoint()
	changed.URL = "https://192.0.2.200"
	if _, err = s.ClaimWebhook(ctx, changed); err != nil {
		t.Fatal(err)
	}
	failed, _, err := s.ReadOperation(ctx, op.ID)
	if err != nil || failed.State != "failed" || failed.Error == nil || failed.Error.Code != "WEBHOOK_ENDPOINT_CHANGED" {
		t.Fatal("endpoint replacement did not fence test", err)
	}
}
