package transport

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
)

type diagnosticAdmission struct {
	taskID             string
	until              time.Time
	terminal, detached bool
}
type admittedDiagnosticRunner struct {
	runner DiagnosticRunner
	permit *diagnosticAdmission
}

func (s *Server) signalAdmissionLocked() {
	if s.admissionChanged != nil {
		close(s.admissionChanged)
	}
	s.admissionChanged = make(chan struct{})
}

// One diagnostic slot is shared by all Monitors; the Agent's other slot
// remains reserved for regular probes. Waiting never creates a physical task.
func (r DiagnosticRunner) Admit(ctx context.Context, action diagnostics.Action, e diagnostics.Executor) (diagnostics.Runner, func(), error) {
	s := r.Server
	for {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		s.mu.Lock()
		session, ok := s.active[e.ID]
		ready := ok && session.appliedRevisions[r.Plan.Revision] && time.Since(session.lastHeartbeat) <= 15*time.Second && session.capacity == 2 && session.capabilities["diagnostics:serial"]
		if action.Check.Runner == "blackbox" && !s.componentReadyLocked(r.Plan, e.ID, time.Now()) {
			ready = false
		}
		if !ready {
			s.mu.Unlock()
			return nil, nil, fmt.Errorf("Agent not ready for diagnostic admission")
		}
		if s.diagnosticAdmissions == nil {
			s.diagnosticAdmissions = map[string]*diagnosticAdmission{}
		}
		current := s.diagnosticAdmissions[e.ID]
		if current != nil && current.detached && !time.Now().Before(current.until) {
			delete(s.diagnosticAdmissions, e.ID)
			current = nil
			s.signalAdmissionLocked()
		}
		if current == nil {
			permit := &diagnosticAdmission{}
			s.diagnosticAdmissions[e.ID] = permit
			s.mu.Unlock()
			var once sync.Once
			release := func() {
				once.Do(func() {
					s.mu.Lock()
					defer s.mu.Unlock()
					if s.diagnosticAdmissions[e.ID] != permit {
						return
					}
					if permit.taskID != "" && !permit.terminal && time.Now().Before(permit.until) {
						// A cancellation or lost reply does not prove the remote probe stopped.
						permit.detached = true
					} else {
						delete(s.diagnosticAdmissions, e.ID)
					}
					s.signalAdmissionLocked()
				})
			}
			return admittedDiagnosticRunner{runner: r, permit: permit}, release, nil
		}
		if s.admissionChanged == nil {
			s.admissionChanged = make(chan struct{})
		}
		changed := s.admissionChanged
		until := current.until
		detached := current.detached
		s.mu.Unlock()
		delay := time.Second // Also recheck heartbeat and session/revision readiness.
		if detached && time.Until(until) < delay {
			delay = max(time.Until(until), time.Millisecond)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, nil, ctx.Err()
		case <-changed:
			timer.Stop()
		case <-timer.C:
		}
	}
}
