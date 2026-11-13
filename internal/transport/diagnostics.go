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
	ctx  context.Context
	task *pb.DiagnosticTask
}
type taskWaiter struct {
	agent  string
	result chan diagnostics.Outcome
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
		out = append(out, diagnostics.Executor{ID: slot.Agent, Zone: slot.Zone, FailureDomain: slot.Zone, Online: true, Capacity: v.capacity, Active: v.active, Supported: caps})
	}
	return out
}

type DiagnosticRunner struct {
	Server *Server
	RunID  string
	Plan   monitor.Plan
}

func (r DiagnosticRunner) Execute(ctx context.Context, a diagnostics.Action, e diagnostics.Executor) (diagnostics.Outcome, error) {
	s := r.Server
	unknown := diagnostics.Outcome{Execution: "execution_failed", Outcome: "unknown"}
	if r.Plan.Diagnostics == nil || !r.Plan.Diagnostics.Enabled {
		return unknown, fmt.Errorf("diagnostics disabled")
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		return unknown, fmt.Errorf("task deadline required")
	}
	s.mu.Lock()
	session, ok := s.active[e.ID]
	s.mu.Unlock()
	if !ok || !session.applied || time.Since(session.lastHeartbeat) > 15*time.Second {
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
	t := storage.DiagnosticTask{ID: id, RunID: r.RunID, Agent: e.ID, Generation: session.generation, Plan: r.Plan, Check: a.Check, Zone: e.Zone, Deadline: deadline}
	if err = s.Store.ReserveDiagnostic(ctx, t); err != nil {
		return unknown, err
	}
	result := make(chan diagnostics.Outcome, 1)
	s.mu.Lock()
	if s.tasks == nil {
		s.tasks = map[string]taskWaiter{}
	}
	s.tasks[id] = taskWaiter{e.ID, result}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.tasks, id); s.mu.Unlock() }()
	task := &pb.DiagnosticTask{TaskId: id, SessionId: r.RunID, Revision: r.Plan.Revision, CompiledCheck: data, ExpiresAtUnixMilli: deadline.UnixMilli(), AttemptNo: 1}
	select {
	case session.tasks <- taskRequest{ctx, task}:
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
	s.mu.Lock()
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
