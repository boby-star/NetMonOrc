package monitor

// FixedPlacement is the resolved controller-owned regular assignment. Empty
// Selected means pending: no partial set is authorized to run regular probes.
type FixedPlacement struct {
	Ref      string
	Hash     string
	Replicas int
	Roster   []Slot
	Selected []Slot
}

func (p Plan) RegularAllowed(agent string) bool {
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
