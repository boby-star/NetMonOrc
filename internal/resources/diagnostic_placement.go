package resources

import (
	"encoding/json"
	"fmt"
	"sort"
)

type diagnosticPlacementBinding struct {
	Ref    string
	Policy PlacementPolicy
}

// Lower only declared placement fields, using the regular PlacementPolicy
// compiler. Source manifests remain unchanged; the profile records ref/hash.
func (r Registry) resolveDiagnosticPlacements(m Manifest) (Manifest, map[string]diagnosticPlacementBinding, error) {
	bindings := map[string]diagnosticPlacementBinding{}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(m.Spec, &spec); err != nil {
		return m, nil, err
	}
	var actions []map[string]json.RawMessage
	if err := json.Unmarshal(spec["actions"], &actions); err != nil {
		return m, nil, err
	}
	for _, action := range actions {
		refRaw, linked := action["placementRef"]
		raw, inline := action["placement"]
		if linked && inline {
			return m, nil, fmt.Errorf("diagnostic action cannot have placement and placementRef")
		}
		if !linked && !inline {
			continue
		}
		var policy PlacementPolicy
		if linked {
			var ref, id string
			if json.Unmarshal(refRaw, &ref) != nil || !namePattern.MatchString(ref) {
				return m, nil, fmt.Errorf("invalid diagnostic placementRef")
			}
			var ok bool
			policy, ok = r.Placements[ref]
			if !ok {
				return m, nil, fmt.Errorf("PlacementPolicy %s is unavailable", ref)
			}
			if json.Unmarshal(action["id"], &id) != nil {
				return m, nil, fmt.Errorf("invalid diagnostic action ID")
			}
			bindings[id] = diagnosticPlacementBinding{ref, policy}
		} else {
			var err error
			policy, err = compilePlacement(raw)
			if err != nil {
				return m, nil, err
			}
		}
		if policy.Replicas != 1 {
			return m, nil, fmt.Errorf("diagnostic placement requires replicas=1; use separate actions for multiple points")
		}
		// Preserve explicit Agent preference for existing inline fixed actions.
		if inline {
			var original struct {
				AgentRefs []string `json:"agentRefs"`
			}
			if err := json.Unmarshal(raw, &original); err != nil {
				return m, nil, err
			}
			policy.AgentRefs = original.AgentRefs
		}
		data, err := json.Marshal(policy)
		if err != nil {
			return m, nil, err
		}
		// Optional nil selector is omitted rather than emitted as forbidden null.
		var fields map[string]json.RawMessage
		json.Unmarshal(data, &fields)
		if string(fields["agentRefs"]) == "null" {
			delete(fields, "agentRefs")
		}
		if string(fields["zones"]) == "null" {
			delete(fields, "zones")
		}
		action["placement"], err = json.Marshal(fields)
		if err != nil {
			return m, nil, err
		}
		delete(action, "placementRef")
	}
	var err error
	spec["actions"], err = json.Marshal(actions)
	if err != nil {
		return m, nil, err
	}
	m.Spec, err = json.Marshal(spec)
	return m, bindings, err
}

// DiagnosticPlacementRefs enumerates only external policy dependencies, for
// bounded on-demand/manual compilation. CompileResource still validates source.
func DiagnosticPlacementRefs(m Manifest) ([]string, error) {
	var spec struct {
		Actions []struct {
			Ref string `json:"placementRef"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(m.Spec, &spec); err != nil {
		return nil, err
	}
	if len(spec.Actions) > 100 {
		return nil, fmt.Errorf("too many diagnostic actions")
	}
	seen := map[string]bool{}
	for _, action := range spec.Actions {
		if action.Ref != "" {
			if !namePattern.MatchString(action.Ref) {
				return nil, fmt.Errorf("invalid placementRef")
			}
			seen[action.Ref] = true
		}
	}
	refs := make([]string, 0, len(seen))
	for ref := range seen {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs, nil
}
