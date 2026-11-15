package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/boby-star/NetMonOrc/internal/webhook"
)

func testAlertRuleResources(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
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
	m, err := resources.Decode([]byte(`{"apiVersion":"netmonorc.io/v1alpha1","kind":"AlertRule","metadata":{"name":"managed-rule"},"spec":{"monitorRef":"service","when":"monitor.state == \"down\"","for":"0s","channelRef":"default-webhook"}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, patch := range []string{`{"monitorRef":"missing","when":"true"}`, `{"monitorRef":"service","when":"true","channelRef":"missing"}`} {
		bad := m
		bad.Spec = json.RawMessage(patch)
		if code, _ := request("POST", "/api/v1/resources/AlertRule", bad, ""); code != 422 {
			t.Fatal("missing alert reference accepted", code)
		}
	}
	code, data := request("POST", "/api/v1/resources/AlertRule", m, "")
	if code != 201 {
		t.Fatalf("create rule: %d %s", code, data)
	}
	var before resources.View
	if err = json.Unmarshal(data, &before); err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(m)
	oldRule, err := config.CompileAlert(data)
	if err != nil {
		t.Fatal(err)
	}
	oldEvaluator, err := alerting.New(oldRule)
	if err != nil {
		t.Fatal(err)
	}
	if code, _ = request("PUT", "/api/v1/resources/AlertRule/managed-rule", m, ""); code != 428 {
		t.Fatal("missing If-Match", code)
	}
	if code, _ = request("PUT", "/api/v1/resources/AlertRule/managed-rule", m, "stale"); code != 412 {
		t.Fatal("stale If-Match", code)
	}
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	spec["severity"] = "critical"
	m.Spec, _ = json.Marshal(spec)
	version := before.ResourceVersion
	code, data = request("POST", "/api/v1/config/apply", resources.Batch{Manifests: []resources.Manifest{m}, ExpectedVersions: map[string]*string{"AlertRule/managed-rule": &version}, RequestID: "managed-rule-replace"}, "")
	if code != 202 {
		t.Fatalf("apply rule: %d %s", code, data)
	}
	var op resources.Operation
	if err = json.Unmarshal(data, &op); err != nil {
		t.Fatal(err)
	}
	a := monitor.Assessment{Monitor: "service", Revision: "alert-resource-test", At: time.Now(), State: "down", Zones: []monitor.ZoneAssessment{{Coverage: "complete", Evidence: []string{"managed-proof"}}}}
	if _, err = s.pool.Exec(ctx, `UPDATE nmo_resources SET effective_revision=$1 WHERE kind='Monitor' AND name='service'`, a.Revision); err != nil {
		t.Fatal(err)
	}
	endpoint := webhook.Endpoint{Name: "default-webhook"}
	if _, err = s.EvaluateAlertResource(ctx, oldEvaluator, endpoint, a, before); !errors.Is(err, resources.ErrVersion) {
		t.Fatal("stale evaluator was not fenced", err)
	}
	if err = s.ReconcileOperations(ctx, "service", a.Revision, nil, nil); err != nil {
		t.Fatal(err)
	}
	pending, _, err := s.Operation(ctx, op.ID)
	if err != nil || pending.State == "completed" {
		t.Fatal("apply completed before evaluation", err)
	}
	current, found, err := s.ResourceKind(ctx, "AlertRule", "managed-rule")
	if err != nil || !found {
		t.Fatal(err)
	}
	data, _ = json.Marshal(current.Manifest)
	rule, err := config.CompileAlert(data)
	if err != nil {
		t.Fatal(err)
	}
	evaluator, err := alerting.New(rule)
	if err != nil {
		t.Fatal(err)
	}
	state, err := s.EvaluateAlertResource(ctx, evaluator, endpoint, a, current)
	if err != nil || state.Status != "firing" {
		t.Fatal("managed rule not evaluated", state.Status, err)
	}
	if err = s.ReconcileOperations(ctx, "service", a.Revision, nil, nil); err != nil {
		t.Fatal(err)
	}
	completed, _, err := s.Operation(ctx, op.ID)
	if err != nil || completed.State != "completed" || completed.Result.Targets[0].Reason != "alert_rule_evaluated" {
		t.Fatal("rule operation not completed", completed.State, err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/AlertRule/managed-rule", nil, current.ResourceVersion); code != 409 {
		t.Fatal("active alert rule deleted", code)
	}
	a.At = a.At.Add(time.Second)
	a.State = "healthy"
	if state, err = s.EvaluateAlertResource(ctx, evaluator, endpoint, a, current); err != nil || state.Status != "resolved" {
		t.Fatal("rule did not resolve", state.Status, err)
	}
	if code, _ = request("DELETE", "/api/v1/resources/AlertRule/managed-rule", nil, current.ResourceVersion); code != 204 {
		t.Fatal("resolved rule not deleted", code)
	}
	if _, err = s.EvaluateAlertResource(ctx, evaluator, endpoint, a, current); !errors.Is(err, resources.ErrVersion) {
		t.Fatal("deleted evaluator not fenced", err)
	}
	if _, err = s.SeedResource(ctx, m, current.Revision, resources.Registry{Active: "service", Zones: []string{"office"}, AlertChannel: "default-webhook"}); !errors.Is(err, resources.ErrNotFound) {
		t.Fatal("deleted bootstrap rule resurrected", err)
	}
}
