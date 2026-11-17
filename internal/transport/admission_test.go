package transport

import (
	"context"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func admissionServer() *Server {
	return &Server{active: map[string]activeSession{"branch": {applied: true, appliedRevisions: map[string]bool{"one": true, "two": true}, lastHeartbeat: time.Now(), capacity: 2, capabilities: map[string]bool{"diagnostics:serial": true}}}}
}
func TestAdmissionSharedAcrossMonitors(t *testing.T) {
	s := admissionServer()
	e := diagnostics.Executor{ID: "branch"}
	first := DiagnosticRunner{Server: s, Plan: monitor.Plan{Revision: "one"}}
	second := DiagnosticRunner{Server: s, Plan: monitor.Plan{Revision: "two"}}
	_, release, err := first.Admit(context.Background(), diagnostics.Action{}, e)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, _, err = second.Admit(ctx, diagnostics.Action{}, e); err == nil {
		t.Fatal("two Monitors reserved the same Agent")
	}
	// A different Agent remains available; regular probe capacity is untouched.
	s.mu.Lock()
	s.active["dc"] = s.active["branch"]
	s.mu.Unlock()
	_, other, err := second.Admit(context.Background(), diagnostics.Action{}, diagnostics.Executor{ID: "dc"})
	if err != nil {
		t.Fatal(err)
	}
	other()
	release()
	release()
	_, next, err := second.Admit(context.Background(), diagnostics.Action{}, e)
	if err != nil {
		t.Fatal(err)
	}
	next()
}
func TestCancelledPhysicalTaskKeepsAdmissionUntilResultOrDeadline(t *testing.T) {
	for _, completion := range []string{"result", "deadline"} {
		t.Run(completion, func(t *testing.T) {
			s := admissionServer()
			r := DiagnosticRunner{Server: s, Plan: monitor.Plan{Revision: "one"}}
			e := diagnostics.Executor{ID: "branch"}
			bound, release, err := r.Admit(context.Background(), diagnostics.Action{}, e)
			if err != nil {
				t.Fatal(err)
			}
			permit := bound.(admittedDiagnosticRunner).permit
			s.mu.Lock()
			permit.taskID = "old-task"
			permit.until = time.Now().Add(50 * time.Millisecond)
			s.mu.Unlock()
			release()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			if _, _, err = r.Admit(ctx, diagnostics.Action{}, e); err == nil {
				t.Fatal("uncertain remote execution overlapped a new task")
			}
			s.taskResult("different-task", "branch", diagnostics.Outcome{})
			if completion == "result" {
				s.taskResult("old-task", "branch", diagnostics.Outcome{})
			}
			ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
			defer cancel2()
			_, done, err := r.Admit(ctx2, diagnostics.Action{}, e)
			if err != nil {
				t.Fatal(err)
			}
			done()
		})
	}
}
