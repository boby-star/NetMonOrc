package transport

import (
	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"sort"
	"time"
)

// AgentStatuses reports the configured roster and actual current sessions.
// Absent telemetry stays null; capability advertisement is not runner health.
func (s *Server) AgentStatuses(now time.Time) []agentstatus.View {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]agentstatus.View, 0, len(s.Slots))
	for _, slot := range s.Slots {
		v := agentstatus.View{AgentID: slot.Agent, Name: slot.Agent, OnlineState: "offline", NetworkZone: slot.Zone, Labels: map[string]string{}}
		if session, ok := s.active[slot.Agent]; ok {
			v.OnlineState = "degraded"
			if session.applied && !session.lastHeartbeat.IsZero() && now.Sub(session.lastHeartbeat) <= 15*time.Second && now.Before(session.certExpiresAt) {
				v.OnlineState = "online"
			}
			for capability, supported := range session.capabilities {
				if supported {
					v.Capabilities = append(v.Capabilities, capability)
				}
			}
			sort.Strings(v.Capabilities)
			if session.applied {
				revision := s.Plan.Revision
				v.AppliedRevision = &revision
			}
			if !session.lastHeartbeat.IsZero() {
				heartbeat := session.lastHeartbeat
				v.LastHeartbeat = &heartbeat
				capacity, active := session.capacity, session.active
				v.MaxConcurrentProbes = &capacity
				v.ActiveProbes = &active
			}
			if !session.certExpiresAt.IsZero() {
				expiry := session.certExpiresAt
				v.CertExpiresAt = &expiry
			}
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentID < out[j].AgentID })
	return out
}
