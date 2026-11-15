// Package dependency validates declared relationships. Edges are context, never
// observations or evidence of a causal failure.
package dependency

import (
	"fmt"
	"regexp"
	"sort"
	"unicode/utf8"
)

type Reference struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

func (r Reference) Key() string { return r.Kind + "/" + r.Name }

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func (r Reference) Valid() bool {
	return (r.Kind == "Target" || r.Kind == "Monitor") && validName.MatchString(r.Name)
}

type Spec struct {
	From        Reference `json:"from"`
	To          Reference `json:"to"`
	Relation    string    `json:"relation"`
	Strength    string    `json:"strength"`
	Description string    `json:"description,omitempty"`
}
type Edge struct {
	Name string `json:"name"`
	Spec
}

func (s *Spec) Normalize() error {
	if s.Relation == "" {
		s.Relation = "requires"
	}
	if s.Strength == "" {
		s.Strength = "soft"
	}
	return s.Validate()
}
func (s Spec) Validate() error {
	if !s.From.Valid() || !s.To.Valid() || s.From == s.To {
		return fmt.Errorf("valid distinct Target/Monitor references required")
	}
	switch s.Relation {
	case "requires", "routesThrough", "hostedOn", "resolvesVia", "relatedTo":
	default:
		return fmt.Errorf("invalid dependency relation")
	}
	if s.Strength != "hard" && s.Strength != "soft" {
		return fmt.Errorf("invalid dependency strength")
	}
	if utf8.RuneCountInString(s.Description) > 1024 {
		return fmt.Errorf("dependency description exceeds 1024 characters")
	}
	return nil
}

// ValidateGraph uses a sparse Kahn traversal; no recursion proportional to graph
// depth and no invented reverse edges. Only requires edges participate in DAG.
func ValidateGraph(edges []Edge, known map[string]bool) error {
	if len(edges) > 4096 {
		return fmt.Errorf("dependency graph exceeds capacity")
	}
	degree := map[string]int{}
	adj := map[string][]string{}
	names := map[string]bool{}
	for _, e := range edges {
		if !validName.MatchString(e.Name) || names[e.Name] {
			return fmt.Errorf("invalid or duplicate dependency name")
		}
		names[e.Name] = true
		if err := e.Spec.Validate(); err != nil {
			return fmt.Errorf("%s: %w", e.Name, err)
		}
		from, to := e.From.Key(), e.To.Key()
		if known != nil && (!known[from] || !known[to]) {
			return fmt.Errorf("%s: dependency reference unavailable", e.Name)
		}
		if e.Relation == "requires" {
			if _, ok := degree[from]; !ok {
				degree[from] = 0
			}
			degree[to]++
			adj[from] = append(adj[from], to)
		}
	}
	queue := []string{}
	for node, n := range degree {
		if n == 0 {
			queue = append(queue, node)
		}
	}
	sort.Strings(queue)
	seen := 0
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		seen++
		for _, next := range adj[node] {
			degree[next]--
			if degree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if seen != len(degree) {
		nodes := []string{}
		for node, n := range degree {
			if n > 0 {
				nodes = append(nodes, node)
			}
		}
		sort.Strings(nodes)
		return fmt.Errorf("requires cycle involving %s", nodes[0])
	}
	return nil
}

// Context returns declared outward reachability, including non-requires cycles
// exactly once. It does not classify a fault or imply shared failure domains.
func Context(edges []Edge, roots ...Reference) []Edge {
	adj := map[string][]Edge{}
	for _, e := range edges {
		adj[e.From.Key()] = append(adj[e.From.Key()], e)
	}
	visited := map[string]bool{}
	queue := []string{}
	for _, r := range roots {
		if r.Valid() && !visited[r.Key()] {
			visited[r.Key()] = true
			queue = append(queue, r.Key())
		}
	}
	out := []Edge{}
	for head := 0; head < len(queue); head++ {
		for _, e := range adj[queue[head]] {
			out = append(out, e)
			next := e.To.Key()
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
