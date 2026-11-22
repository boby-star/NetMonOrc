package resources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/dependency"
)

// MonitorDependencies lowers DSL §15 shorthand to Monitor -> Target requires.
// It leaves the persisted source manifest intact, including the input list order.
func monitorDependencyRefs(m Manifest) (Manifest, []string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(m.Spec, &fields); err != nil || fields == nil {
		return m, nil, fmt.Errorf("Monitor.spec requires an object")
	}
	raw, present := fields["dependencies"]
	if !present {
		return m, nil, nil
	}
	var refs []string
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, &refs) != nil || len(refs) > 4096 {
		return m, nil, fmt.Errorf("Monitor.spec.dependencies requires at most 4096 Target names")
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		if !namePattern.MatchString(ref) || seen[ref] {
			return m, nil, fmt.Errorf("Monitor.spec.dependencies requires unique valid Target names")
		}
		seen[ref] = true
	}
	sort.Strings(refs)
	delete(fields, "dependencies")
	m.Spec, _ = json.Marshal(fields)
	return m, refs, nil
}

func requirementKey(from, to dependency.Reference) string { return from.Key() + "->" + to.Key() }

func derivedRequirements(m Manifest, refs []string, edges []dependency.Edge, targets map[string]TargetSpec) ([]dependency.Edge, error) {
	existing := map[string]bool{}
	for _, e := range edges {
		if e.Relation == "requires" {
			existing[requirementKey(e.From, e.To)] = true
		}
	}
	from := dependency.Reference{Kind: "Monitor", Name: m.Metadata.Name}
	out := append([]dependency.Edge(nil), edges...)
	for _, ref := range refs {
		if _, known := targets[ref]; !known {
			return nil, fmt.Errorf("Monitor/%s: dependencies Target/%s is unavailable", m.Metadata.Name, ref)
		}
		to := dependency.Reference{Kind: "Target", Name: ref}
		key := requirementKey(from, to)
		if existing[key] {
			continue
		}
		sum := sha256.Sum256([]byte(key))
		out = append(out, dependency.Edge{Name: "inline-" + hex.EncodeToString(sum[:])[:52], Spec: dependency.Spec{From: from, To: to, Relation: "requires", Strength: "soft"}})
		existing[key] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (r Registry) lowerMonitorDependencies(m Manifest) (Registry, Manifest, error) {
	lowered, refs, err := monitorDependencyRefs(m)
	if err != nil {
		return r, m, err
	}
	if len(refs) == 0 {
		return r, lowered, nil
	}
	r.Dependencies, err = derivedRequirements(m, refs, r.Dependencies, r.Targets)
	if err == nil {
		err = dependency.ValidateGraph(r.Dependencies, nil)
	}
	return r, lowered, err
}

// DependencyContext is a read-only view of the resolved desired graph. Derived
// links retain the owning Monitor instead of masquerading as stored resources.
type DependencyContextLink struct {
	Name string `json:"name"`
	dependency.Spec
	Derived    bool   `json:"derived"`
	DeclaredBy string `json:"declaredBy"`
}
type DependencyContext struct {
	Monitor        string                  `json:"monitor"`
	ConfigRevision string                  `json:"configRevision"`
	Items          []DependencyContextLink `json:"items"`
}
