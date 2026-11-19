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

func TestDiagnosticEstimatedBytesAtomicReservation(t *testing.T) {
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
	name := "byte-test-" + newID()
	p := monitor.Plan{Name: name, Revision: "byte-revision", Diagnostics: &monitor.DiagnosticBinding{Enabled: true, ProfileRef: "bytes", ProfileHash: "hash"}}
	profile := diagnostics.Profile{Mode: "adaptive", Budget: diagnostics.Budget{MaxChecks: 10, MaxDuration: time.Minute, MaxEstimatedBytes: 8192}}
	run, err := s.StartDiagnostic(ctx, p, profile)
	if err != nil || run == "" {
		t.Fatal(run, err)
	}
	gen, err := s.BeginSession(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Assign(ctx, name, gen, p.Revision, "office", name); err != nil {
		t.Fatal(err)
	}
	if err = s.Applied(ctx, name, gen, p.Revision); err != nil {
		t.Fatal(err)
	}
	task := DiagnosticTask{RunID: run, Agent: name, Generation: gen, Plan: p, Zone: "office", Deadline: time.Now().Add(5 * time.Second), Check: monitor.Check{ID: "dns", Type: "dns", Runner: "blackbox", DNS: &monitor.DNSConfig{}}}
	// Two concurrent callers can afford only one physical attempt.
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		copy := task
		copy.ID = newID()
		go func() { results <- s.ReserveDiagnostic(ctx, copy) }()
	}
	accepted := 0
	for i := 0; i < 2; i++ {
		if <-results == nil {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatal("non-atomic estimated-byte reservation", accepted)
	}
	view, found, err := s.DiagnosticSession(ctx, run)
	if err != nil || !found || view.BudgetUsed.AdditionalChecks != 1 || view.BudgetUsed.EstimatedBytes == nil || *view.BudgetUsed.EstimatedBytes != 8192 {
		t.Fatalf("public budget: %+v %v", view, err)
	}
	tasks, err := s.DiagnosticTasks(ctx, run, "", 10)
	if err != nil || len(tasks) != 1 || tasks[0].EstimatedBytes == nil || *tasks[0].EstimatedBytes != 8192 {
		t.Fatal("physical reservation estimate missing", tasks, err)
	}
	// A rejected task consumes its original reservation; there is no refund.
	if err = s.DiagnosticReceipt(ctx, tasks[0].ID, name, gen, false); err != nil {
		t.Fatal(err)
	}
	task.ID = newID()
	if err = s.ReserveDiagnostic(ctx, task); err == nil {
		t.Fatal("rejection refunded byte budget")
	}
	if err = s.FinishDiagnostic(ctx, run, diagnostics.Result{Mode: "adaptive", Status: "inconclusive", Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	report, err := s.LatestDiagnostic(ctx, name)
	if err != nil || report.ReservedEstimatedBytes == nil || *report.ReservedEstimatedBytes != 8192 {
		t.Fatal("durable estimate lost", report, err)
	}
}
