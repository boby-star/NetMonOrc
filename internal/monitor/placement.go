package monitor

import (
	"slices"
	"sort"
)

// FixedPlacement is the resolved controller-owned regular assignment. Empty
// Selected means pending: no partial set is authorized to run regular probes.
type FixedPlacement struct {
	ScopeZones     []string          `json:",omitempty"`
	FailureDomains map[string]string `json:",omitempty"`
	DistinctBy     string            `json:",omitempty"`
	Strategy       string            `json:",omitempty"`
	Candidates     []Slot            `json:",omitempty"`
	Ref            string
	Hash           string
	Replicas       int
	Roster         []Slot
	Selected       []Slot
}

func (p Plan) RegularAllowed(agent string) bool {
	if p.Disabled {
		return false
	}
	if len(p.CheckPlacements) > 0 {
		for _, check := range p.Checks {
			if p.CheckAllowed(check.ID, agent) {
				return true
			}
		}
		return false
	}
	if p.FixedPlacement == nil {
		return true
	}
	for _, slot := range p.FixedPlacement.Selected {
		if slot.Agent == agent {
			return true
		}
	}
	return false
}
func (p Plan) RegularSlots(roster []Slot) []Slot {
	if len(p.CheckPlacements) > 0 {
		known := map[string]Slot{}
		for _, check := range p.Checks {
			for _, slot := range p.CheckSlots(check.ID, roster) {
				known[slot.Agent] = slot
			}
		}
		out := []Slot{}
		for _, slot := range known {
			out = append(out, slot)
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Agent < out[j].Agent })
		return out
	}

	if p.FixedPlacement == nil {
		return append([]Slot(nil), roster...)
	}
	return append([]Slot(nil), p.FixedPlacement.Selected...)
}
func (p Plan) AssessmentSlots(roster []Slot) []Slot {
	required := map[string]bool{}
	for _, zone := range p.Zones {
		required[zone] = true
	}
	out := []Slot{}
	for _, slot := range p.RegularSlots(roster) {
		if required[slot.Zone] {
			out = append(out, slot)
		}
	}
	return out
}

// ResolvedPlacement is Controller-owned state, never a YAML resource.
type ResolvedPlacement struct {
	PolicyHash   string
	TemplateHash string
	Selected     []Slot
}

// ReplicaSources includes potential same-zone sources of a dynamic plan, so
// deferred CEL value aggregation cannot become active after a later failover.
func (p Plan) ReplicaSources() []Slot {
	if p.FixedPlacement == nil {
		return nil
	}
	if p.FixedPlacement.Strategy != "" && p.FixedPlacement.Replicas > 1 && p.FixedPlacement.DistinctBy != "zone" {
		return p.FixedPlacement.Candidates
	}
	return p.FixedPlacement.Selected
}

func (p Plan) CheckAllowed(check, agent string) bool {
	if p.Disabled {
		return false
	}
	binding := p.CheckPlacements[check]
	if binding == nil {
		binding = p.FixedPlacement
	}
	if binding == nil {
		return true
	}
	for _, slot := range binding.Selected {
		if slot.Agent == agent {
			return true
		}
	}
	return false
}
func (p Plan) CheckSlots(check string, roster []Slot) []Slot {
	binding := p.CheckPlacements[check]
	if binding == nil {
		binding = p.FixedPlacement
	}
	if binding == nil {
		return append([]Slot(nil), roster...)
	}
	return append([]Slot(nil), binding.Selected...)
}
func (p Plan) CheckInZone(check, zone string) bool {
	b := p.CheckPlacements[check]
	return b == nil || len(b.ScopeZones) == 0 || slices.Contains(b.ScopeZones, zone)
}
func (p Plan) HasDynamicPlacement() bool {
	if p.FixedPlacement != nil && p.FixedPlacement.Strategy != "" {
		return true
	}
	for _, binding := range p.CheckPlacements {
		if binding.Strategy != "" {
			return true
		}
	}
	return false
}

// CouldRunCheck gates capability requirements before dynamic selection exists.
func (p Plan) CouldRunCheck(check, agent string) bool {
	binding := p.CheckPlacements[check]
	if binding == nil {
		binding = p.FixedPlacement
	}
	if binding == nil {
		return true
	}
	sources := binding.Selected
	if binding.Strategy != "" {
		sources = binding.Candidates
	}
	for _, slot := range sources {
		if slot.Agent == agent {
			return true
		}
	}
	return false
}
