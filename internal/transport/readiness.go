package transport

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"time"
)

// AppliedAssignments is independent of diagnostic capabilities and capacities.
func (s *Server) AppliedAssignments(now time.Time) []monitor.AppliedAssignment {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []monitor.AppliedAssignment{}
	for _, slot := range s.Slots {
		session, ok := s.active[slot.Agent]
		if !ok || !session.applied || session.lastHeartbeat.IsZero() || now.Sub(session.lastHeartbeat) > 15*time.Second {
			continue
		}
		out = append(out, monitor.AppliedAssignment{Agent: slot.Agent, Zone: slot.Zone, Generation: session.generation, Revision: s.Plan.Revision})
	}
	return out
}
