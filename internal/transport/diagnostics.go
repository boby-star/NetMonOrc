package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/storage"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
)

type taskRequest struct {
	response *pb.ResponseCommand
	cancel   *pb.TaskCancel
	ctx      context.Context
	task     *pb.DiagnosticTask
}
type taskWaiter struct {
	agent  string
	result chan diagnostics.ObservedOutcome
}

func (s *Server) Executors(now time.Time) []diagnostics.Executor {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []diagnostics.Executor
	for _, slot := range s.Slots {
		v, ok := s.active[slot.Agent]
		if !ok || !v.applied || now.Sub(v.lastHeartbeat) > 15*time.Second || !v.capabilities["diagnostics:serial"] || v.capacity != 2 {
			continue
		}
		caps := map[string]bool{}
		for k, b := range v.capabilities {
			caps[k] = b
		}
		knownAt := time.Time{}
		if s.FailureDomains[slot.Agent] != "" {
			knownAt = s.FailureDomainsKnownAt
		}
		out = append(out, diagnostics.Executor{Labels: s.AgentLabels[slot.Agent], EvidenceNotBefore: knownAt, ID: slot.Agent, Zone: slot.Zone, FailureDomain: s.FailureDomains[slot.Agent], Online: true, Capacity: v.capacity, Active: v.active, Supported: caps})
	}
	return out
}

type DiagnosticRunner struct {
	Fallback *storage.FallbackRun
	Server   *Server
	RunID    string
	Plan     monitor.Plan
}

func (r DiagnosticRunner) Execute(ctx context.Context, a diagnostics.Action, e diagnostics.Executor) (diagnostics.Outcome, error) {
	runner, release, err := r.Admit(ctx, a, e)
	if err != nil {
		return diagnostics.Outcome{Execution: "execution_failed", Outcome: "unknown"}, err
	}
	defer release()
	return runner.Execute(ctx, a, e)
}
func (bound admittedDiagnosticRunner) Execute(ctx context.Context, a diagnostics.Action, e diagnostics.Executor) (diagnostics.Outcome, error) {
	observed, err := bound.ExecuteEvidence(ctx, a, e)
	return observed.Outcome, err
}
func (r DiagnosticRunner) ExecuteEvidence(ctx context.Context, a diagnostics.Action, e diagnostics.Executor) (diagnostics.ObservedOutcome, error) {
	runner, release, err := r.Admit(ctx, a, e)
	if err != nil {
		return diagnostics.ObservedOutcome{Outcome: diagnostics.Outcome{Execution: "execution_failed", Outcome: "unknown"}}, err
	}
	defer release()
	return runner.(diagnostics.EvidenceRunner).ExecuteEvidence(ctx, a, e)
}
func (bound admittedDiagnosticRunner) ExecuteEvidence(ctx context.Context, a diagnostics.Action, e diagnostics.Executor) (diagnostics.ObservedOutcome, error) {
	r := bound.runner
	s := r.Server
	unknown := diagnostics.ObservedOutcome{Outcome: diagnostics.Outcome{Execution: "execution_failed", Outcome: "unknown"}}
	if a.AttemptNo < 1 || a.AttemptNo > 50 {
		return unknown, fmt.Errorf("invalid diagnostic attempt number")
	}
	if r.Fallback == nil && (r.Plan.Diagnostics == nil || !r.Plan.Diagnostics.Enabled) {
		return unknown, fmt.Errorf("diagnostics disabled")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return unknown, fmt.Errorf("task deadline required")
	}
	s.mu.Lock()
	session, ok := s.active[e.ID]
	ready := ok && session.appliedRevisions[r.Plan.Revision] && time.Since(session.lastHeartbeat) <= 15*time.Second
	s.mu.Unlock()
	if !ready {
		return unknown, fmt.Errorf("Agent not ready")
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return unknown, err
	}
	id := hex.EncodeToString(b[:])
	data, err := json.Marshal(a.Check)
	if err != nil || len(data) > 64<<10 {
		return unknown, fmt.Errorf("invalid compiled diagnostic check")
	}
	s.mu.Lock()
	permit := bound.permit
	if s.diagnosticAdmissions[e.ID] != permit || permit.detached || (a.Check.Runner == "blackbox" && !s.componentReadyLocked(r.Plan, e.ID, time.Now())) {
		s.mu.Unlock()
		return unknown, fmt.Errorf("diagnostic admission expired")
	}
	permit.taskID, permit.until, permit.terminal = id, deadline, false
	s.mu.Unlock()
	t := storage.DiagnosticTask{StepID: a.StepID, ID: id, RunID: r.RunID, Agent: e.ID, Generation: session.generation, Plan: r.Plan, Check: a.Check, Zone: e.Zone, Deadline: deadline, AttemptNo: a.AttemptNo}
	if r.Fallback != nil {
		err = s.Store.ReserveFallback(ctx, t)
	} else {
		err = s.Store.ReserveDiagnostic(ctx, t)
	}
	if err != nil {
		return unknown, err
	}
	defer func() {
		if ctx.Err() != nil {
			select {
			case session.tasks <- taskRequest{ctx: context.Background(), cancel: &pb.TaskCancel{TaskId: id, Reason: "controller_cancelled"}}:
			default:
			}
		}
	}()
	result := make(chan diagnostics.ObservedOutcome, 1)
	s.mu.Lock()
	if s.tasks == nil {
		s.tasks = map[string]taskWaiter{}
	}
	s.tasks[id] = taskWaiter{e.ID, result}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.tasks, id); s.mu.Unlock() }()
	task := &pb.DiagnosticTask{TaskId: id, SessionId: r.RunID, Revision: r.Plan.Revision, CompiledCheck: data, ExpiresAtUnixMilli: deadline.UnixMilli(), AttemptNo: uint32(a.AttemptNo)}
	if r.Fallback != nil {
		task.FallbackRunId = r.RunID
		task.FallbackOriginAgent = r.Fallback.OriginAgent
		task.FallbackOriginZone = r.Fallback.OriginZone
	}
	select {
	case session.tasks <- taskRequest{ctx: ctx, task: task}:
	case <-ctx.Done():
		return unknown, ctx.Err()
	}
	select {
	case o := <-result:
		return o, nil
	case <-ctx.Done():
		return unknown, ctx.Err()
	}
}
func (s *Server) taskResult(id, agent string, o diagnostics.Outcome) {
	s.taskEvidenceResult(id, agent, diagnostics.ObservedOutcome{Outcome: o})
}
func (s *Server) taskEvidenceResult(id, agent string, o diagnostics.ObservedOutcome) {
	s.mu.Lock()
	if permit := s.diagnosticAdmissions[agent]; permit != nil && permit.taskID == id {
		permit.terminal = true
		if permit.detached {
			delete(s.diagnosticAdmissions, agent)
			s.signalAdmissionLocked()
		}
	}
	w, ok := s.tasks[id]
	s.mu.Unlock()
	if !ok || w.agent != agent {
		return
	}
	select {
	case w.result <- o:
	default:
	}
}
