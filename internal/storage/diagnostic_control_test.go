package storage

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func testDiagnosticControl(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client) {
	t.Helper()
	var raw string
	parsed, _ := url.Parse(server.URL)
	for _, cookie := range client.Jar.Cookies(parsed) {
		if cookie.Name == "netmonorc_session" {
			raw = cookie.Value
		}
	}
	identity, err := auth.New(s).Me(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	actor := auth.AdminCredential{UserID: identity.UserID, Hash: auth.HashToken(raw)}
	data, err := os.ReadFile("../../deployment/lab/adaptive-confirmed.yaml")
	if err != nil {
		t.Fatal(err)
	}
	profileManifest, err := resources.DecodeYAML(data)
	if err != nil {
		t.Fatal(err)
	}
	profileManifest.Metadata.Name = "manual-profile"
	registry := resources.Registry{}
	compiled, err := registry.CompileResource(profileManifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SeedResource(ctx, profileManifest, compiled.Revision); err != nil {
		t.Fatal(err)
	}
	m, err := resources.Decode([]byte(testMonitor))
	if err != nil {
		t.Fatal(err)
	}
	m.Metadata.Name = "manual-monitor"
	var spec map[string]any
	_ = json.Unmarshal(m.Spec, &spec)
	spec["diagnostics"] = map[string]any{"enabled": true, "profileRef": "manual-profile", "cooldown": "60s"}
	m.Spec, _ = json.Marshal(spec)
	registry.Profiles = map[string]diagnostics.Profile{compiled.Profile.Name: *compiled.Profile}
	p, err := registry.Compile(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SeedResource(ctx, m, p.Revision); err != nil {
		t.Fatal(err)
	}
	a := monitor.Assessment{Monitor: p.Name, Revision: p.Revision, At: time.Now(), State: "healthy", Zones: []monitor.ZoneAssessment{{Zone: "office", Coverage: "complete"}}}
	if err = s.SaveAssessment(ctx, a); err != nil {
		t.Fatal(err)
	}
	request := diagnosticview.Start{Monitor: p.Name, ProfileRef: "manual-profile", Reason: "manual", RequestID: "manual-one"}
	id, err := s.StartManualDiagnostic(ctx, p, *compiled.Profile, actor, request)
	if err != nil || id == "" {
		t.Fatal("manual admission", err)
	}
	if replay, err := s.StartManualDiagnostic(ctx, p, *compiled.Profile, actor, request); err != nil || replay != id {
		t.Fatal("start replay", err)
	}
	changed := request
	changed.Reason = "trigger"
	if _, err = s.StartManualDiagnostic(ctx, p, *compiled.Profile, actor, changed); !errors.Is(err, resources.ErrIdempotency) {
		t.Fatal("idempotency body reuse", err)
	}
	generation, err := s.BeginSession(ctx, "manual-agent")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Assign(ctx, "manual-agent", generation, p.Revision, "office", p.Name); err != nil {
		t.Fatal(err)
	}
	if err = s.Applied(ctx, "manual-agent", generation, p.Revision); err != nil {
		t.Fatal(err)
	}
	task := DiagnosticTask{ID: newID(), RunID: id, Agent: "manual-agent", Generation: generation, Plan: p, Check: compiled.Profile.Actions[0].Check, Zone: "office", Deadline: time.Now().Add(5 * time.Second), AttemptNo: 1}
	if err = s.ReserveDiagnostic(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err = s.CancelDiagnostic(ctx, actor, id); err != nil {
		t.Fatal(err)
	}
	if err = s.CancelDiagnostic(ctx, actor, id); err != nil {
		t.Fatal("cancel replay", err)
	}
	if err = s.DiagnosticReceipt(ctx, task.ID, task.Agent, generation, true); err != nil {
		t.Fatal("late receipt broke stream", err)
	}
	now := time.Now().UTC()
	observation := monitor.Observation{ID: task.ID, DiagnosticTaskID: task.ID, Agent: task.Agent, Zone: task.Zone, Monitor: p.Name, Revision: p.Revision, Check: task.Check.ID, StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "failure"}
	if _, err = s.CommitBatch(ctx, task.Agent, generation, p.Revision, []monitor.Observation{observation}); err != nil {
		t.Fatal("late result lost", err)
	}
	view, found, err := s.DiagnosticSession(ctx, id)
	if err != nil || !found || view.State != "cancelled" || view.BudgetUsed.AdditionalChecks != 1 || len(view.EvidenceIDs) != 1 {
		t.Fatal("cancel changed budget/evidence", err)
	}
	tasks, err := s.DiagnosticTasks(ctx, id, "", 20)
	if err != nil || len(tasks) != 1 || tasks[0].State != "cancelled" {
		t.Fatal("late result undid task cancellation", err)
	}
	if err = s.FinishDiagnostic(ctx, id, diagnostics.Result{Status: "completed"}); !errors.Is(err, ErrDiagnosticTerminal) {
		t.Fatal("late planner overwrote cancelled", err)
	}
	second := request
	second.RequestID = "manual-two"
	if next, err := s.StartManualDiagnostic(ctx, p, *compiled.Profile, actor, second); err != nil || next != "" {
		t.Fatal("cooldown bypassed", err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE nmo_incidents SET last_finished=now()-interval '61 seconds' WHERE monitor=$1`, p.Name); err != nil {
		t.Fatal(err)
	}
	a.State = "down"
	a.At = time.Now()
	if err = s.SaveAssessment(ctx, a); err != nil {
		t.Fatal(err)
	}
	if next, err := s.StartManualDiagnostic(ctx, p, *compiled.Profile, actor, second); err != nil || next != "" {
		t.Fatal("cancellation reset incident", err)
	}
	a.State = "healthy"
	a.At = time.Now()
	if err = s.SaveAssessment(ctx, a); err != nil {
		t.Fatal(err)
	}
	next, err := s.StartManualDiagnostic(ctx, p, *compiled.Profile, actor, second)
	if err != nil || next == "" || next == id {
		t.Fatal("healthy manual episode blocked", err)
	}
	if err = s.CancelDiagnostic(ctx, actor, next); err != nil {
		t.Fatal(err)
	}
	// Former runtime cannot create another run after desired revision changes.
	if _, err = s.pool.Exec(ctx, `UPDATE nmo_resources SET effective_revision='newer' WHERE kind='Monitor' AND name=$1`, p.Name); err != nil {
		t.Fatal(err)
	}
	third := request
	third.RequestID = "manual-three"
	if _, err = s.StartManualDiagnostic(ctx, p, *compiled.Profile, actor, third); !errors.Is(err, resources.ErrConflict) {
		t.Fatal("stale runtime launched work", err)
	}
	if replay, err := s.RecoverDiagnosticStart(ctx, actor, request); err != nil || replay != id {
		t.Fatal("recovery after revision change", err)
	}
}
