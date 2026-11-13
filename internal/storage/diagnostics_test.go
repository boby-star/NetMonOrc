package storage

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestPostgresDiagnosticBudgetsAndIsolation(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires real PostgreSQL DSN file")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, e := Open(ctx, strings.TrimSpace(string(b)))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	name := "diagnostic-test-" + newID()
	p := monitor.Plan{Name: name, Revision: "revision", MaxAge: time.Minute, Diagnostics: &monitor.DiagnosticBinding{Enabled: true, ProfileRef: "test", ProfileHash: "hash", Cooldown: time.Millisecond}}
	profile := diagnostics.Profile{Mode: "adaptive", Budget: diagnostics.Budget{MaxChecks: 1, MaxDuration: time.Minute}}
	run, e := s.StartDiagnostic(ctx, p, profile)
	if e != nil || run == "" {
		t.Fatalf("start: %s %v", run, e)
	}
	gen, e := s.BeginSession(ctx, name)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Assign(ctx, name, gen, p.Revision, "office", p.Name); e != nil {
		t.Fatal(e)
	}
	if e = s.Applied(ctx, name, gen, p.Revision); e != nil {
		t.Fatal(e)
	}
	task := DiagnosticTask{ID: newID(), RunID: run, Agent: name, Generation: gen, Plan: p, Check: monitor.Check{ID: "http"}, Zone: "office", Deadline: time.Now().Add(5 * time.Second)}
	if e = s.ReserveDiagnostic(ctx, task); e != nil {
		t.Fatal(e)
	}
	second := task
	second.ID = newID()
	if e = s.ReserveDiagnostic(ctx, second); e == nil {
		t.Fatal("second reservation exceeded durable budget")
	}
	now := time.Now().UTC()
	o := monitor.Observation{ID: task.ID, DiagnosticTaskID: task.ID, Agent: name, Zone: "office", Monitor: name, Revision: p.Revision, Check: "http", StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "failure"}
	bad := o
	bad.Check = "forged"
	if _, e = s.CommitBatch(ctx, name, gen, p.Revision, []monitor.Observation{bad}); e == nil {
		t.Fatal("unbound diagnostic result accepted")
	}
	if _, e = s.CommitBatch(ctx, name, gen, p.Revision, []monitor.Observation{o}); e != nil {
		t.Fatal(e)
	}
	duplicate, e := s.CommitBatch(ctx, name, gen, p.Revision, []monitor.Observation{o})
	if e != nil || !duplicate[0] {
		t.Fatal("diagnostic replay not idempotent", e)
	}
	latest, e := s.Latest(ctx, p, now)
	if e != nil || len(latest) != 0 {
		t.Fatal("diagnostic replaced regular observation", e)
	}
	if e = s.FinishDiagnostic(ctx, run, diagnostics.Result{Mode: "adaptive", Status: "inconclusive", Reason: "test"}); e != nil {
		t.Fatal(e)
	}
	p.Revision = "new-revision"
	if id, e := s.StartDiagnostic(ctx, p, profile); e != nil || id != "" {
		t.Fatal("revision reset incident admission", id, e)
	}
	a := monitor.Assessment{Monitor: name, State: "healthy", Zones: []monitor.ZoneAssessment{{Coverage: "coverage_degraded"}}}
	if e = s.CloseDiagnosticEpisode(ctx, a); e != nil {
		t.Fatal(e)
	}
	if id, e := s.StartDiagnostic(ctx, p, profile); e != nil || id != "" {
		t.Fatal("partial healthy closed incident", id, e)
	}
	a.Zones[0].Coverage = "complete"
	if e = s.CloseDiagnosticEpisode(ctx, a); e != nil {
		t.Fatal(e)
	}
	// Move the clock anchor in this test instead of sleeping past cooldown.
	if _, e = s.pool.Exec(ctx, `UPDATE nmo_incidents SET last_finished=now()-interval '1 minute' WHERE monitor=$1`, name); e != nil {
		t.Fatal(e)
	}
	next, e := s.StartDiagnostic(ctx, p, profile)
	if e != nil || next == "" || next == run {
		t.Fatal("new healthy-to-failure episode blocked", e)
	}
	var used int
	var saved []byte
	if e = s.pool.QueryRow(ctx, `SELECT used_checks,report FROM nmo_diagnostic_runs WHERE id=$1`, run).Scan(&used, &saved); e != nil || used != 1 || !json.Valid(saved) {
		t.Fatal("durable budget/report corrupted", e)
	}
}
