package resources

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

type PlacementPolicy struct {
	AgentRefs            []string `json:"agentRefs"`
	Zones                []string `json:"zones"`
	Replicas             int      `json:"replicas"`
	DistinctBy           string   `json:"distinctBy"`
	Strategy             string   `json:"strategy"`
	OnInsufficientAgents string   `json:"onInsufficientAgents"`
}

var placementAgentID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func (p PlacementPolicy) Hash() string { return valueHash(p) }
func compilePlacement(spec json.RawMessage) (PlacementPolicy, error) {
	var wire struct {
		AgentRefs            []string `json:"agentRefs"`
		Zones                []string `json:"zones"`
		Replicas             *int     `json:"replicas"`
		DistinctBy           string   `json:"distinctBy"`
		Strategy             string   `json:"strategy"`
		OnInsufficientAgents string   `json:"onInsufficientAgents"`
		Selector             struct {
			MatchLabels      map[string]string `json:"matchLabels"`
			MatchExpressions []json.RawMessage `json:"matchExpressions"`
		} `json:"selector"`
	}
	if err := closedJSON(spec, &wire); err != nil {
		return PlacementPolicy{}, err
	}
	p := PlacementPolicy{AgentRefs: wire.AgentRefs, Zones: wire.Zones, Replicas: 1, DistinctBy: wire.DistinctBy, Strategy: wire.Strategy, OnInsufficientAgents: wire.OnInsufficientAgents}
	if wire.Replicas != nil {
		p.Replicas = *wire.Replicas
	}
	if p.Strategy == "" {
		p.Strategy = "leastLoaded"
	}
	if p.DistinctBy == "" {
		p.DistinctBy = "none"
	}
	if p.OnInsufficientAgents == "" {
		p.OnInsufficientAgents = "pending"
	}
	if p.Strategy != "fixed" && p.Strategy != "leastLoaded" && p.Strategy != "roundRobin" {
		return p, fmt.Errorf("invalid placement strategy")
	}
	if p.OnInsufficientAgents != "pending" {
		return p, fmt.Errorf("partial regular placement is disabled")
	}
	if p.DistinctBy != "none" && p.DistinctBy != "zone" {
		return p, fmt.Errorf("failureDomain placement requires a trusted domain registry")
	}
	if len(wire.Selector.MatchLabels) > 0 || len(wire.Selector.MatchExpressions) > 0 {
		return p, fmt.Errorf("regular selectors require Controller-managed Agent labels")
	}
	if p.Replicas < 1 || p.Replicas > 10 || (p.Strategy == "fixed" && len(p.AgentRefs) < p.Replicas) || len(p.AgentRefs) > 100 {
		return p, fmt.Errorf("placement requires 1..10 replicas; fixed requires sufficient agentRefs")
	}
	seen := map[string]bool{}
	for _, id := range p.AgentRefs {
		if !placementAgentID.MatchString(id) || seen[id] {
			return p, fmt.Errorf("invalid or duplicate placement Agent ref")
		}
		seen[id] = true
	}
	seen = map[string]bool{}
	for _, zone := range p.Zones {
		if zone == "" || len(zone) > 63 || seen[zone] {
			return p, fmt.Errorf("invalid or duplicate placement zone")
		}
		seen[zone] = true
	}
	sort.Strings(p.AgentRefs)
	sort.Strings(p.Zones)
	return p, nil
}
func (r Registry) resolvePlacement(m Manifest) (Manifest, *monitor.FixedPlacement, error) {
	var spec map[string]json.RawMessage
	if json.Unmarshal(m.Spec, &spec) != nil || spec == nil {
		return m, nil, fmt.Errorf("Monitor.spec requires an object")
	}
	inline, hasInline := spec["placement"]
	refRaw, linked := spec["placementRef"]
	if !hasInline && !linked {
		return m, nil, nil
	}
	if hasInline && linked {
		return m, nil, fmt.Errorf("Monitor cannot contain both placement and placementRef")
	}
	var policy PlacementPolicy
	var ref string
	if linked {
		if json.Unmarshal(refRaw, &ref) != nil || !namePattern.MatchString(ref) {
			return m, nil, fmt.Errorf("invalid placementRef")
		}
		var found bool
		policy, found = r.Placements[ref]
		if !found {
			return m, nil, fmt.Errorf("PlacementPolicy %s is unavailable", ref)
		}
	} else {
		var err error
		policy, err = compilePlacement(inline)
		if err != nil {
			return m, nil, err
		}
	}
	if len(r.Roster) == 0 || len(r.Roster) > 100 {
		return m, nil, fmt.Errorf("placement requires a trusted Controller roster")
	}
	known := map[string]monitor.Slot{}
	rosterZones := map[string]bool{}
	for _, slot := range r.Roster {
		if !placementAgentID.MatchString(slot.Agent) || slot.Zone == "" || len(slot.Zone) > 63 || rosterZones[slot.Zone] {
			return m, nil, fmt.Errorf("fixed roster requires one Agent per valid zone")
		}
		if _, duplicate := known[slot.Agent]; duplicate {
			return m, nil, fmt.Errorf("duplicate Controller roster Agent")
		}
		known[slot.Agent] = slot
		rosterZones[slot.Zone] = true
	}
	zones := map[string]bool{}
	for _, zone := range policy.Zones {
		zones[zone] = true
	}
	candidates := []monitor.Slot{}
	refs := policy.AgentRefs
	if len(refs) == 0 {
		for id := range known {
			refs = append(refs, id)
		}
		sort.Strings(refs)
	}
	for _, id := range refs {
		slot, registered := known[id]
		if !registered {
			return m, nil, fmt.Errorf("placement Agent %s is not registered in the Controller roster", id)
		}
		if len(zones) == 0 || zones[slot.Zone] {
			candidates = append(candidates, slot)
		}
	}
	selected := []monitor.Slot{}
	if policy.Strategy == "fixed" && len(candidates) >= policy.Replicas {
		selected = append(selected, candidates[:policy.Replicas]...)
	}
	if policy.Strategy != "fixed" {
		if previous, ok := r.ResolvedPlacements[m.Metadata.Name]; ok && previous.PolicyHash == policy.Hash() {
			permitted := map[string]monitor.Slot{}
			for _, slot := range candidates {
				permitted[slot.Agent] = slot
			}
			if len(previous.Selected) != 0 && len(previous.Selected) != policy.Replicas {
				return m, nil, fmt.Errorf("partial runtime placement is forbidden")
			}
			for _, slot := range previous.Selected {
				if permitted[slot.Agent] != slot {
					return m, nil, fmt.Errorf("runtime placement source is not permitted")
				}
			}
			selected = append(selected, previous.Selected...)
		}
	}
	// The current assessor supports one expected source per zone. This is explicit
	// admission, not an approximation of same-zone replica voting.
	seenZones := map[string]bool{}
	for _, slot := range selected {
		if seenZones[slot.Zone] {
			return m, nil, fmt.Errorf("same-zone regular replicas are not implemented")
		}
		seenZones[slot.Zone] = true
	}
	roster := append([]monitor.Slot(nil), r.Roster...)
	sort.Slice(roster, func(i, j int) bool { return roster[i].Agent < roster[j].Agent })
	binding := &monitor.FixedPlacement{Ref: ref, Hash: policy.Hash(), Replicas: policy.Replicas, Roster: roster, Selected: selected}
	if policy.Strategy != "fixed" {
		binding.Strategy = policy.Strategy
		binding.Candidates = candidates
	}
	delete(spec, "placement")
	delete(spec, "placementRef")
	m.Spec, _ = json.Marshal(spec)
	return m, binding, nil
}
