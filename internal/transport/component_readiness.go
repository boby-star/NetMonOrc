package transport

import (
	"fmt"
	"time"

	"github.com/boby-star/NetMonOrc/internal/component"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
)

func validateComponentReports(agent string, plans []monitor.Plan, applied map[string]bool, reports []*pb.ComponentReadiness, now time.Time) (map[string]component.Readiness, error) {
	if len(reports) > 100 {
		return nil, fmt.Errorf("too many Component readiness reports")
	}
	known := map[string]monitor.Plan{}
	for _, p := range plans {
		known[p.Revision] = p
	}
	result := map[string]component.Readiness{}
	seen := map[string]bool{}
	for _, report := range reports {
		if report == nil || seen[report.Revision] || !applied[report.Revision] {
			return nil, fmt.Errorf("unbound or duplicate Component readiness")
		}
		seen[report.Revision] = true
		p, current := known[report.Revision]
		if !current {
			continue
		} // In-flight heartbeat from a withdrawn applied Plan.
		binding, explicit := p.Components[agent]
		at := time.UnixMilli(report.CheckedAtUnixMilli)
		if !explicit || report.ComponentRef != binding.Ref || report.BindingHash != binding.Hash || !component.Fresh(at, now) {
			return nil, fmt.Errorf("invalid Component readiness binding or timestamp")
		}
		valid := report.State == "disabled" && report.Reason == "binding_disabled" && !binding.Enabled
		if binding.Enabled {
			valid = (report.State == "ready" && report.Reason == "endpoint_ready") || (report.State == "unavailable" && report.Reason == "endpoint_unavailable")
		}
		if !valid {
			return nil, fmt.Errorf("invalid Component readiness state")
		}
		result[report.Revision] = component.Readiness{Revision: report.Revision, Ref: report.ComponentRef, Hash: report.BindingHash, State: report.State, Reason: report.Reason, CheckedAt: at}
	}
	return result, nil
}

// componentReadyLocked uses current session telemetry, not the planner's frozen
// executor catalog. Call under mu, including immediately before task reservation.
func (s *Server) componentReadyLocked(p monitor.Plan, agent string, now time.Time) bool {
	b, explicit := p.Components[agent]
	if !explicit {
		return true
	}
	if !b.Enabled {
		return false
	}
	v, connected := s.active[agent]
	r, reported := v.components[p.Revision]
	return connected && v.appliedRevisions[p.Revision] && now.Sub(v.lastHeartbeat) <= 15*time.Second && now.Before(v.certExpiresAt) && reported && r.Ref == b.Ref && r.Hash == b.Hash && r.State == "ready" && component.Fresh(r.CheckedAt, now)
}

func (s *Server) ComponentReady(p monitor.Plan, agent string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.componentReadyLocked(p, agent, now)
}

func (s *Server) componentStatusesLocked(agent string, now time.Time) []component.Status {
	var result []component.Status
	for _, p := range s.plans() {
		b, explicit := p.Components[agent]
		if !explicit {
			continue
		}
		v, connected := s.active[agent]
		status := component.Status{MonitorID: p.Name, Revision: p.Revision, ComponentRef: b.Ref, State: "unknown", Reason: "not_reported"}
		if !b.Enabled {
			status.State = "disabled"
			status.Reason = "binding_disabled"
		}
		if r, ok := v.components[p.Revision]; ok && r.Ref == b.Ref && r.Hash == b.Hash {
			at := r.CheckedAt
			status.CheckedAt = &at
			if b.Enabled {
				status.State = "unknown"
				status.Reason = "stale_report"
				if connected && v.appliedRevisions[p.Revision] && now.Sub(v.lastHeartbeat) <= 15*time.Second && now.Before(v.certExpiresAt) && component.Fresh(r.CheckedAt, now) {
					status.State = r.State
					status.Reason = r.Reason
				}
			}
		}
		result = append(result, status)
	}
	return result
}
