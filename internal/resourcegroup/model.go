// Package resourcegroup derives group state from current Monitor assessments.
// It never runs probes or copies observations into a new stream.
package resourcegroup

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"time"
)

type Expression struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}
type Selector struct {
	MatchLabels      map[string]string `json:"matchLabels,omitempty"`
	MatchExpressions []Expression      `json:"matchExpressions,omitempty"`
}
type Evaluation struct {
	Strategy    string             `json:"strategy"`
	MinCoverage *float64           `json:"minCoverage,omitempty"`
	Weights     map[string]float64 `json:"weights,omitempty"`
	HealthyMin  *float64           `json:"healthyMin,omitempty"`
}
type Spec struct {
	Members     *[]string   `json:"members,omitempty"`
	Selector    *Selector   `json:"selector,omitempty"`
	Mode        string      `json:"mode"`
	Evaluation  *Evaluation `json:"evaluation,omitempty"`
	Description string      `json:"description,omitempty"`
}

var name = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func (s *Spec) Validate() error {
	if (s.Members == nil) == (s.Selector == nil) {
		return fmt.Errorf("exactly one of members/selector is required")
	}
	if s.Mode == "" {
		s.Mode = "context"
	}
	if s.Mode != "context" && s.Mode != "derived" {
		return fmt.Errorf("invalid group mode")
	}
	if s.Members != nil {
		seen := map[string]bool{}
		for _, v := range *s.Members {
			if !name.MatchString(v) || seen[v] {
				return fmt.Errorf("invalid or duplicate Monitor member")
			}
			seen[v] = true
		}
		sort.Strings(*s.Members)
	}
	if s.Selector != nil {
		for _, e := range s.Selector.MatchExpressions {
			if e.Key == "" {
				return fmt.Errorf("selector key is required")
			}
			switch e.Operator {
			case "In", "NotIn":
				if len(e.Values) == 0 {
					return fmt.Errorf("selector values required")
				}
			case "Exists", "DoesNotExist":
				if len(e.Values) != 0 {
					return fmt.Errorf("selector values forbidden")
				}
			default:
				return fmt.Errorf("invalid selector operator")
			}
		}
	}
	if s.Mode == "context" {
		if s.Evaluation != nil {
			return fmt.Errorf("context groups cannot have evaluation")
		}
		return nil
	}
	e := s.Evaluation
	if e == nil {
		return fmt.Errorf("derived group requires evaluation")
	}
	if e.MinCoverage == nil {
		v := 1.
		e.MinCoverage = &v
	}
	if !weight(*e.MinCoverage) {
		return fmt.Errorf("invalid minCoverage")
	}
	switch e.Strategy {
	case "all-required", "any":
		if e.Weights != nil || e.HealthyMin != nil {
			return fmt.Errorf("weights/healthyMin require weighted strategy")
		}
	case "weighted":
		if s.Members == nil || len(e.Weights) != len(*s.Members) {
			return fmt.Errorf("weighted requires explicit weights for every fixed member")
		}
		var sum float64
		for _, id := range *s.Members {
			w, ok := e.Weights[id]
			if !ok || !weight(w) {
				return fmt.Errorf("invalid member weight")
			}
			sum += w
		}
		if sum <= 0 {
			return fmt.Errorf("positive weight sum required")
		}
		// Q04: absent threshold requires score=1. An explicit DSL threshold wins.
		if e.HealthyMin == nil {
			v := 1.
			e.HealthyMin = &v
		}
		if !weight(*e.HealthyMin) {
			return fmt.Errorf("invalid healthyMin")
		}
	default:
		return fmt.Errorf("invalid group strategy")
	}
	return nil
}
func weight(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
func (s Selector) Match(labels map[string]string) bool {
	for k, v := range s.MatchLabels {
		if actual, ok := labels[k]; !ok || actual != v {
			return false
		}
	}
	for _, e := range s.MatchExpressions {
		v, exists := labels[e.Key]
		in := false
		for _, allowed := range e.Values {
			in = in || v == allowed
		}
		switch e.Operator {
		case "In":
			if !exists || !in {
				return false
			}
		case "NotIn":
			if exists && in {
				return false
			}
		case "Exists":
			if !exists {
				return false
			}
		case "DoesNotExist":
			if exists {
				return false
			}
		default:
			return false
		}
	}
	return true
}

type Member struct {
	MonitorID     string `json:"monitorId"`
	State         string `json:"state"`
	CoverageState string `json:"coverageState"`
	Revision      string `json:"revision"`
}
type Assessment struct {
	GroupName     string   `json:"groupName"`
	Mode          string   `json:"mode"`
	State         *string  `json:"state"`
	CoverageState string   `json:"coverageState"`
	Members       []Member `json:"members"`
	EvidenceIDs   []string `json:"evidenceIds"`
}
type Source struct {
	Member      Member
	At          time.Time
	EvidenceIDs []string
}

// Evaluate requires already revision-validated sources from a consistent snapshot.
func Evaluate(group string, s Spec, ids []string, sources map[string]Source, now time.Time) Assessment {
	out := Assessment{GroupName: group, Mode: s.Mode, CoverageState: "unknown", Members: []Member{}, EvidenceIDs: []string{}}
	var total, valid, score float64
	states := map[string]int{}
	seen := map[string]bool{}
	allCoverage := true
	for _, id := range ids {
		src, ok := sources[id]
		m := src.Member
		m.MonitorID = id
		if !ok || src.At.After(now) || now.Sub(src.At) > 5*time.Second || !validState(m.State) {
			m.State = "unknown"
			m.CoverageState = "unknown"
		}
		w := 1.
		if s.Evaluation != nil && s.Evaluation.Strategy == "weighted" {
			w = s.Evaluation.Weights[id]
		}
		total += w
		if m.State != "unknown" {
			valid += w
			switch m.State {
			case "healthy":
				score += w
			case "degraded":
				score += .5 * w
			}
			for _, e := range src.EvidenceIDs {
				if !seen[e] {
					out.EvidenceIDs = append(out.EvidenceIDs, e)
					seen[e] = true
				}
			}
		}
		allCoverage = allCoverage && m.State != "unknown" && m.CoverageState == "sufficient"
		states[m.State]++
		out.Members = append(out.Members, m)
	}
	if len(ids) > 0 {
		out.CoverageState = "degraded"
		if allCoverage {
			out.CoverageState = "sufficient"
		} else if states["unknown"] == len(ids) {
			out.CoverageState = "unknown"
		}
	}
	if s.Mode == "context" {
		out.CoverageState = "unknown"
		return out
	}
	state := "unknown"
	out.State = &state
	if len(ids) == 0 || total == 0 || valid/total < *s.Evaluation.MinCoverage {
		return out
	}
	switch s.Evaluation.Strategy {
	case "weighted":
		if valid == 0 {
			return out
		}
		h := score / valid
		switch {
		case h >= *s.Evaluation.HealthyMin:
			state = "healthy"
		case h == 0:
			state = "down"
		default:
			state = "degraded"
		}
	case "any":
		if states["healthy"] > 0 {
			state = "healthy"
		} else if states["down"] == len(ids) {
			state = "down"
		} else if states["down"] > 0 || states["degraded"] > 0 {
			state = "degraded"
		}
	case "all-required":
		if states["healthy"] == len(ids) {
			state = "healthy"
		} else if states["down"] == len(ids) {
			state = "down"
		} else if states["down"] > 0 || states["degraded"] > 0 {
			state = "degraded"
		}
	}
	return out
}
func validState(s string) bool {
	return s == "healthy" || s == "degraded" || s == "down" || s == "unknown"
}
