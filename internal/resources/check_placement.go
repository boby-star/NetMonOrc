package resources

import (
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"sort"
)

// splitCheckPlacement validates the oneOf independently of live inventory.
// The remainder is still checked by the closed protocol/check compiler.
func splitCheckPlacement(raw json.RawMessage) (json.RawMessage, map[string]json.RawMessage, error) {
	var check map[string]json.RawMessage
	if err := closedJSON(raw, &check); err != nil || check == nil {
		return nil, nil, fmt.Errorf("check requires an object")
	}
	placement, inline := check["placement"]
	ref, linked := check["placementRef"]
	if inline && linked {
		return nil, nil, fmt.Errorf("check cannot contain both placement and placementRef")
	}
	binding := map[string]json.RawMessage{}
	if inline {
		if _, err := compilePlacement(placement); err != nil {
			return nil, nil, err
		}
		binding["placement"] = placement
	}
	if linked {
		var name string
		if json.Unmarshal(ref, &name) != nil || !namePattern.MatchString(name) {
			return nil, nil, fmt.Errorf("invalid check placementRef")
		}
		binding["placementRef"] = ref
	}
	delete(check, "placement")
	delete(check, "placementRef")
	remainder, err := json.Marshal(check)
	return remainder, binding, err
}

func (r Registry) resolveCheckPlacements(m Manifest) (Manifest, map[string]*monitor.FixedPlacement, error) {
	var spec map[string]json.RawMessage
	if json.Unmarshal(m.Spec, &spec) != nil {
		return m, nil, fmt.Errorf("invalid Monitor spec")
	}
	checks := []json.RawMessage{}
	if len(spec["checks"]) > 0 && (string(spec["checks"]) == "null" || json.Unmarshal(spec["checks"], &checks) != nil) {
		return m, nil, fmt.Errorf("checks require a list")
	}
	bindings := map[string]*monitor.FixedPlacement{}
	for i, raw := range checks {
		rest, placement, err := splitCheckPlacement(raw)
		if err != nil {
			return m, nil, fmt.Errorf("checks[%d]: %w", i, err)
		}
		if len(placement) == 0 {
			continue
		}
		var header struct {
			ID      string `json:"id"`
			Enabled *bool  `json:"enabled"`
		}
		if json.Unmarshal(rest, &header) != nil || !checkIdentifier.MatchString(header.ID) {
			return m, nil, fmt.Errorf("invalid check ID")
		}
		virtual := m
		virtual.Metadata.Name = m.Metadata.Name + "/" + header.ID
		virtual.Spec, _ = json.Marshal(placement)
		_, resolved, err := r.resolvePlacement(virtual)
		if err != nil {
			return m, nil, fmt.Errorf("check %s: %w", header.ID, err)
		}
		// Scope follows the actual selection, or the possible scope while pending.
		scope := resolved.Selected
		if len(scope) == 0 {
			scope = resolved.Candidates
		}
		if len(scope) == 0 {
			scope = resolved.Roster
		}
		zones := map[string]bool{}
		for _, slot := range scope {
			zones[slot.Zone] = true
		}
		for zone := range zones {
			resolved.ScopeZones = append(resolved.ScopeZones, zone)
		}
		sort.Strings(resolved.ScopeZones)
		if header.Enabled == nil || *header.Enabled {
			bindings[header.ID] = resolved
		}
		checks[i] = rest
	}
	if len(bindings) == 0 {
		bindings = nil
	}
	spec["checks"], _ = json.Marshal(checks)
	m.Spec, _ = json.Marshal(spec)
	return m, bindings, nil
}
