package transport

import (
	"context"
	"errors"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/storage"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
)

func ResponseEligible(p monitor.Plan, a monitor.Assessment) bool {
	if p.Disabled || a.Revision != p.Revision || a.Monitor != p.Name || (a.State != "down" && a.State != "degraded") || len(a.Zones) == 0 || time.Since(a.At) > 5*time.Second || a.At.After(time.Now().Add(time.Second)) {
		return false
	}
	for _, zone := range a.Zones {
		if zone.Coverage != "complete" || len(zone.Evidence) == 0 {
			return false
		}
	}
	return true
}

// Called only after the exact Assessment was durably saved by Controller.
func (s *Server) Respond(ctx context.Context, p monitor.Plan, a monitor.Assessment) error {
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if len(p.Responses) == 0 || p.Disabled || a.Monitor != p.Name || a.Revision != p.Revision || time.Since(a.At) > 5*time.Second || a.At.After(time.Now().Add(time.Second)) {
		return nil
	}
	if a.State == "healthy" && a.Revision == p.Revision && monitor.MonitorContext(p, a).CoverageState == "sufficient" {
		return s.Store.ResetResponses(ctx, p)
	}
	if !ResponseEligible(p, a) {
		return nil
	}
	if err := s.Store.ControllerAlive(ctx); err != nil {
		return err
	}
	for _, rule := range p.Responses {
		condition, err := diagnostics.CompileTrigger(rule.When, p)
		if err != nil {
			return err
		}
		yes, err := condition(ctx, a)
		if err != nil || !yes {
			continue
		}
		s.mu.Lock()
		session, ok := s.active[rule.AgentRef]
		ready := ok && session.capabilities["response:block-port-v1"] && session.appliedRevisions[p.Revision] && time.Since(session.lastHeartbeat) <= 15*time.Second
		s.mu.Unlock()
		if !ready {
			continue
		}
		event, err := s.Store.ReserveResponse(ctx, p, a, rule, session.generation)
		if errors.Is(err, storage.ErrFenced) {
			continue
		}
		if err != nil {
			return err
		}
		if event == nil {
			continue
		}
		command := &pb.ResponseCommand{Id: event.ID, Revision: p.Revision, RuleId: rule.ID, AcceptUntilUnixMilli: event.AcceptUntil.UnixMilli(), BlockUntilUnixMilli: event.BlockUntil.UnixMilli()}
		select {
		case session.tasks <- taskRequest{ctx: parent, response: command}:
		default:
			if err = s.Store.ResponseReceipt(ctx, event.ID, rule.AgentRef, session.generation, "failed", "dispatch_busy"); err != nil && !errors.Is(err, storage.ErrFenced) {
				return err
			}
		}
	}
	return nil
}
