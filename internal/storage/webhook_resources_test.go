package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/httpapi"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/boby-star/NetMonOrc/internal/webhook"
)

func testWebhookResources(t *testing.T, ctx context.Context, s *Store, h *httpapi.Handler, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
	allowed := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	registry := resources.Registry{Active: "service", Zones: []string{"office"}, ManagedWebhooks: true, WebhookCIDRs: allowed}
	secretDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(secretDir, "local-token"), []byte("local-test-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	s.ConfigureManagedWebhooks(func(e webhook.Endpoint) error {
		sender, err := webhook.New(e, allowed, secretDir, "")
		if err != nil {
			return err
		}
		sender.Close()
		return nil
	})
	h.SetResources(resources.New(s, registry), "")
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
	makeEndpoint := func(name string, enabled bool, ref string) resources.Manifest {
		spec, _ := json.Marshal(map[string]any{"url": "https://127.0.0.1:9444/events", "enabled": enabled, "auth": map[string]string{"type": "bearer", "secretRef": ref}})
		return resources.Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "WebhookEndpoint", Metadata: resources.Metadata{Name: name}, Spec: spec}
	}
	blocked := makeEndpoint("outside-policy", false, "local-token")
	blocked.Spec = json.RawMessage(`{"url":"https://192.0.2.10:9444/events","enabled":false}`)
	if code, _ := request("POST", "/api/v1/resources/WebhookEndpoint", blocked, ""); code != 422 {
		t.Fatal("outside-CIDR endpoint admitted", code)
	}
	active := makeEndpoint("default-webhook", true, "local-token")
	code, data := request("POST", "/api/v1/resources/WebhookEndpoint", active, "")
	if code != 201 {
		t.Fatalf("create endpoint %d %s", code, data)
	}
	var first resources.View
	if err := json.Unmarshal(data, &first); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("local-test-secret")) {
		t.Fatal("credential leaked")
	}
	if code, _ = request("PUT", "/api/v1/resources/WebhookEndpoint/default-webhook", active, ""); code != 428 {
		t.Fatal("If-Match missing", code)
	}
	if code, _ = request("PUT", "/api/v1/resources/WebhookEndpoint/default-webhook", active, "stale"); code != 412 {
		t.Fatal("stale If-Match", code)
	}
	if code, _ = request("POST", "/api/v1/resources/WebhookEndpoint", makeEndpoint("second-active", true, "local-token"), ""); code != 422 {
		t.Fatal("two enabled endpoints admitted", code)
	}
	inactive := makeEndpoint("standby-webhook", false, "not-installed")
	code, data = request("POST", "/api/v1/resources/WebhookEndpoint", inactive, "")
	if code != 201 {
		t.Fatal("inactive replacement rejected", code)
	}
	var standby resources.View
	_ = json.Unmarshal(data, &standby)
	off := makeEndpoint("default-webhook", false, "local-token")
	onMissing := makeEndpoint("standby-webhook", true, "not-installed")
	a, b := first.ResourceVersion, standby.ResourceVersion
	batch := resources.Batch{Manifests: []resources.Manifest{off, onMissing}, ExpectedVersions: map[string]*string{"WebhookEndpoint/default-webhook": &a, "WebhookEndpoint/standby-webhook": &b}, RequestID: "webhook-missing-secret"}
	if code, _ = request("POST", "/api/v1/config/apply", batch, ""); code != 422 {
		t.Fatal("missing credential admitted", code)
	}
	unchanged, _, err := s.ResourceKind(ctx, "WebhookEndpoint", "default-webhook")
	if err != nil || unchanged.ResourceVersion != a {
		t.Fatal("failed preflight partially committed", err)
	}
	oldEndpoint := webhook.Endpoint{}
	result, err := registry.CompileResource(active)
	if err != nil {
		t.Fatal(err)
	}
	oldEndpoint = *result.Endpoint
	batch.Manifests = []resources.Manifest{off, makeEndpoint("standby-webhook", true, "local-token")}
	batch.RequestID = "webhook-switch"
	code, data = request("POST", "/api/v1/config/apply", batch, "")
	if code != 202 {
		t.Fatalf("atomic channel switch %d %s", code, data)
	}
	var op resources.Operation
	_ = json.Unmarshal(data, &op)
	rows, err := s.WebhookResources(ctx)
	if err != nil || len(rows) != 2 {
		t.Fatal("endpoint catalog", err)
	}
	proofs := map[string]string{}
	var current webhook.Endpoint
	for _, v := range rows {
		entry, e := registry.CompileResource(v.Manifest)
		if e != nil {
			t.Fatal(e)
		}
		proofs[v.Manifest.Metadata.Name] = v.UID + "/" + entry.Revision
		if entry.Endpoint.Enabled {
			current = *entry.Endpoint
		}
	}
	var rawCookie string
	u, _ := url.Parse(server.URL)
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == "netmonorc_session" {
			rawCookie = c.Value
		}
	}
	actor := auth.AdminCredential{UserID: session.UserID, Hash: auth.HashToken(rawCookie)}
	queued, err := s.QueueWebhookTest(ctx, actor, current)
	if err != nil {
		t.Fatal(err)
	}
	if job, err := s.ClaimWebhook(ctx, oldEndpoint); err != nil || job != nil {
		t.Fatal("stale sender acquired delivery", err)
	}
	var state string
	if err = s.pool.QueryRow(ctx, `SELECT o.state FROM nmo_webhook_outbox o JOIN nmo_webhook_tests t ON t.event_id=o.event_id WHERE t.operation_id=$1`, queued.ID).Scan(&state); err != nil || state != "pending" {
		t.Fatal("stale sender cancelled current event", state, err)
	}
	if _, err = s.QueueWebhookTest(ctx, actor, oldEndpoint); err != resources.ErrConflict {
		t.Fatal("stale endpoint accepted test", err)
	}
	if err = s.ReconcileOperations(ctx, "service", "", nil, nil); err != nil {
		t.Fatal(err)
	}
	pending, _, err := s.Operation(ctx, op.ID)
	if err != nil || pending.State == "completed" {
		t.Fatal("endpoint apply ready before runtime", err)
	}
	if err = s.ReconcileOperations(ctx, "service", "", nil, nil, proofs); err != nil {
		t.Fatal(err)
	}
	done, _, err := s.Operation(ctx, op.ID)
	if err != nil || done.State != "completed" {
		t.Fatal("loaded endpoint operation not completed", done.State, err)
	}
	// A disabled, unreferenced replacement can be deleted with its exact ETag.
	v, _, err := s.ResourceKind(ctx, "WebhookEndpoint", "default-webhook")
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/WebhookEndpoint/default-webhook", nil, v.ResourceVersion); code != 204 {
		t.Fatal("delete unreferenced endpoint", code)
	}
	if _, err = s.SeedResource(ctx, active, first.Revision, registry); err != resources.ErrNotFound {
		t.Fatal("deleted endpoint resurrected", err)
	}
}
