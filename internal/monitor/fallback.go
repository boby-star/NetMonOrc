package monitor

import "slices"

type FallbackPolicy struct {
	On          []string
	Strategy    string
	MaxAttempts int
}

func FallbackCause(o Observation) string {
	if o.DiagnosticTaskID != "" || o.FallbackTaskID != "" || o.Execution != "execution_failed" || o.Outcome != "unknown" {
		return ""
	}
	switch o.Reason {
	case "runner_unavailable", "exporter_request_failed":
		return "runner_unavailable"
	case "local_execution_error", "invalid_exporter_metrics", "response_read_failed", "icmp_execution_unavailable":
		return "local_execution_error"
	}
	return ""
}

// FallbackCandidate admits only spare physical sources. A source already
// supplying this check cannot become an extra independent vote for a replica.
func (p Plan) FallbackCandidate(c Check, origin, candidate Slot, domains map[string]string) bool {
	if c.Fallback == nil || p.Disabled || candidate.Agent == origin.Agent {
		return false
	}
	if !slices.Contains(p.FallbackRoster, candidate) || !slices.Contains(p.CheckSlots(c.ID, p.FallbackRoster), origin) {
		return false
	}
	if c.Fallback.Strategy == "same-zone" && candidate.Zone != origin.Zone || c.Fallback.Strategy == "different-zone" && candidate.Zone == origin.Zone {
		return false
	}
	for _, source := range p.CheckSlots(c.ID, p.FallbackRoster) {
		if source.Agent == candidate.Agent {
			return false
		}
	}
	b := p.CheckPlacements[c.ID]
	if b == nil {
		b = p.FixedPlacement
	}
	if b != nil && b.DistinctBy == "failureDomain" {
		domain := domains[candidate.Agent]
		if domain == "" {
			return false
		}
		for _, source := range p.CheckSlots(c.ID, p.FallbackRoster) {
			if source.Agent != origin.Agent && (domains[source.Agent] == "" || domains[source.Agent] == domain) {
				return false
			}
		}
	}
	return true
}

// SourceAgent is Controller-enriched assessment context, never Agent testimony.
func (o Observation) SourceAgent() string {
	if o.FallbackForAgent != "" {
		return o.FallbackForAgent
	}
	return o.Agent
}
