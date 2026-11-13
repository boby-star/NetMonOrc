package storage

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/webhook"
)

func TestPostgresWebhookSummaryIsolation(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires isolated real PostgreSQL")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := Open(ctx, strings.TrimSpace(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	name := "summary-" + newID()
	endpoint := webhook.Endpoint{Name: name, URL: "https://192.0.2.1", Enabled: true, MaxAttempts: 2}
	view, err := s.WebhookSummary(ctx, endpoint, name, name)
	if err != nil || view.Latest != nil || view.Pending != 0 {
		t.Fatal("empty queue", view, err)
	}
	for _, ruleName := range []string{name, name + "-other"} {
		rule, err := alerting.New(alerting.Rule{Name: ruleName, Monitor: name, Channel: name, Scope: "monitor", When: `monitor.state == "down"`, ResolveWhen: `monitor.state == "healthy"`, Enabled: true, NotifyEvents: []string{"firing"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.EvaluateAlert(ctx, rule, endpoint, monitor.Assessment{Monitor: name, Revision: "r", At: time.Now().UTC(), State: "down"}); err != nil {
			t.Fatal(err)
		}
	}
	view, err = s.WebhookSummary(ctx, endpoint, name, name)
	if err != nil || view.Pending != 1 || view.Latest == nil || view.Latest.State != "pending" || view.Latest.Attempts != 0 {
		t.Fatal("enqueue not isolated", view, err)
	}
	id := view.Latest.ID
	for _, state := range []string{"leased", "delivered", "failed", "cancelled"} {
		_, err = s.pool.Exec(ctx, "UPDATE nmo_webhook_outbox SET state=$2,attempts=2,status_code=503 WHERE event_id=$1", id, state)
		if err != nil {
			t.Fatal(err)
		}
		view, err = s.WebhookSummary(ctx, endpoint, name, name)
		if err != nil || view.Pending != 0 || view.Latest.State != state || view.Latest.Attempts != 2 || view.Latest.StatusCode != 503 {
			t.Fatal("stale delivery status", view, err)
		}
		if view.Leased+view.Delivered+view.Failed+view.Cancelled != 1 {
			t.Fatal("inconsistent counters", view)
		}
	}
	endpoint.URL = "https://192.0.2.2"
	view, err = s.WebhookSummary(ctx, endpoint, name, name)
	if err != nil || view.Latest != nil {
		t.Fatal("old endpoint revision leaked", view, err)
	}
}
