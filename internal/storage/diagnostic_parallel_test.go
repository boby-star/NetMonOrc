package storage

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestDiagnosticParallelReservationIsAtomic(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires real PostgreSQL DSN file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, err := Open(ctx, strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	name := "parallel-" + newID()
	p := monitor.Plan{Name: name, Revision: "parallel-revision", Diagnostics: &monitor.DiagnosticBinding{Enabled: true, ProfileRef: "parallel", ProfileHash: "hash"}}
	profile := diagnostics.Profile{Mode: "fixed", Budget: diagnostics.Budget{MaxChecks: 3, MaxDuration: time.Minute, MaxParallel: 1}}
	run, err := s.StartDiagnostic(ctx, p, profile)
	if err != nil || run == "" {
		t.Fatal(run, err)
	}
	type result struct {
		task DiagnosticTask
		err  error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, suffix := range []string{"a", "b"} {
		agent := name + "-" + suffix
		gen, e := s.BeginSession(ctx, agent)
		if e != nil {
			t.Fatal(e)
		}
		if e = s.Assign(ctx, agent, gen, p.Revision, "office", name); e != nil {
			t.Fatal(e)
		}
		if e = s.Applied(ctx, agent, gen, p.Revision); e != nil {
			t.Fatal(e)
		}
		task := DiagnosticTask{ID: newID(), RunID: run, Agent: agent, Generation: gen, Plan: p, Zone: "office", Deadline: time.Now().Add(5 * time.Second), Check: monitor.Check{ID: "http"}, StepID: "node-" + suffix}
		go func() { <-start; results <- result{task, s.ReserveDiagnostic(ctx, task)} }()
	}
	close(start)
	accepted := 0
	var winner DiagnosticTask
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err == nil {
			accepted++
			winner = r.task
		}
	}
	if accepted != 1 {
		t.Fatal("concurrent callers exceeded maxParallel", accepted)
	}
	if err = s.DiagnosticReceipt(ctx, winner.ID, winner.Agent, winner.Generation, false); err != nil {
		t.Fatal(err)
	}
	winner.ID = newID()
	winner.StepID = "next-node"
	if err = s.ReserveDiagnostic(ctx, winner); err != nil {
		t.Fatal("terminal task retained active parallel slot", err)
	}
	tasks, err := s.DiagnosticTasks(ctx, run, "", 10)
	if err != nil || len(tasks) != 2 {
		t.Fatal(tasks, err)
	}
	for _, task := range tasks {
		if task.StepID == "" {
			t.Fatal("physical step provenance lost")
		}
	}
	var used int
	if err = s.pool.QueryRow(ctx, `SELECT used_checks FROM nmo_diagnostic_runs WHERE id=$1`, run).Scan(&used); err != nil || used != 2 {
		t.Fatal("parallel slot release refunded physical budget", used, err)
	}
	if err = s.FinishDiagnostic(ctx, run, diagnostics.Result{Mode: "fixed", Status: "inconclusive", Reason: "test"}); err != nil {
		t.Fatal(err)
	}
}
