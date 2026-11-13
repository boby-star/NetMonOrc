// Package config compiles a deliberately restricted, closed subset of v1alpha1.
package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"gopkg.in/yaml.v3"
)

type Manifest struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Target struct {
			Address string `yaml:"address"`
		} `yaml:"target"`
		Checks      []checkSpec            `yaml:"checks"`
		Assessment  assessmentSpec         `yaml:"assessment"`
		Diagnostics *diagnosticBindingSpec `yaml:"diagnostics"`
	} `yaml:"spec"`
}
type checkSpec struct {
	Extract  []extractSpec `yaml:"extract"`
	ID       string        `yaml:"id"`
	Type     string        `yaml:"type"`
	Runner   string        `yaml:"runner"`
	Interval string        `yaml:"interval"`
	Timeout  string        `yaml:"timeout"`
	Config   yaml.Node     `yaml:"config"`
}
type assessmentSpec struct {
	Rules []struct {
		ID       string `yaml:"id"`
		Priority *int   `yaml:"priority"`
		When     string `yaml:"when"`
		Then     struct {
			State  string `yaml:"state"`
			Reason string `yaml:"reason"`
		} `yaml:"then"`
	} `yaml:"rules"`
	RequiredZones   []string `yaml:"requiredZones"`
	ZoneAggregation string   `yaml:"zoneAggregation"`
	MinCoverage     *float64 `yaml:"minCoverage"`
	Freshness       struct {
		MaxAge  string `yaml:"maxAge"`
		MaxSkew string `yaml:"maxSkew"`
	} `yaml:"freshness"`
	Checks []struct {
		ID       string   `yaml:"id"`
		Weight   *float64 `yaml:"weight"`
		Required bool     `yaml:"required"`
	} `yaml:"checks"`
	Weighted struct {
		Enabled     *bool    `yaml:"enabled"`
		HealthyMin  *float64 `yaml:"healthyMin"`
		DegradedMin *float64 `yaml:"degradedMin"`
	} `yaml:"weighted"`
}

var namePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var durationPattern = regexp.MustCompile(`^[0-9]+(ms|s|m|h)$`)

func duration(s string, fallback time.Duration) (time.Duration, error) {
	if s == "" {
		return fallback, nil
	}
	if !durationPattern.MatchString(s) {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	d, e := time.ParseDuration(s)
	if e != nil || d <= 0 {
		return 0, fmt.Errorf("duration must be positive: %q", s)
	}
	return d, nil
}
func number(v *float64, fallback float64) (float64, error) {
	if v == nil {
		return fallback, nil
	}
	if math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 1 {
		return 0, fmt.Errorf("weight/threshold must be finite and within 0..1")
	}
	return *v, nil
}
func Compile(data []byte) (monitor.Plan, error) {
	p := monitor.Plan{}
	if len(data) > 1<<20 {
		return p, fmt.Errorf("manifest exceeds 1 MiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	var m Manifest
	var tree yaml.Node
	if e := yaml.Unmarshal(data, &tree); e != nil {
		return p, e
	}
	if len(tree.Content) != 1 {
		return p, fmt.Errorf("empty Monitor document")
	}
	if e := scalarTypes(tree.Content[0], reflect.TypeOf(m)); e != nil {
		return p, e
	}
	if e := d.Decode(&m); e != nil {
		return p, e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return p, fmt.Errorf("this slice accepts exactly one Monitor document")
	}
	if m.APIVersion != "netmonorc.io/v1alpha1" || m.Kind != "Monitor" || !namePattern.MatchString(m.Metadata.Name) {
		return p, fmt.Errorf("invalid apiVersion, kind or metadata.name")
	}
	if m.Spec.Target.Address == "" {
		return p, fmt.Errorf("spec.target.address is required")
	}
	p.Name = m.Metadata.Name
	p.TargetAddress = m.Spec.Target.Address
	if m.Spec.Diagnostics != nil {
		var err error
		p.Diagnostics, err = compileBinding(*m.Spec.Diagnostics)
		if err != nil {
			return p, err
		}
	}
	a := m.Spec.Assessment
	if a.ZoneAggregation != "" && a.ZoneAggregation != "all-required" {
		return p, fmt.Errorf("only all-required zoneAggregation is implemented")
	}
	if a.Weighted.Enabled != nil && !*a.Weighted.Enabled {
		return p, fmt.Errorf("weighted.enabled=false is not implemented")
	}
	var e error
	if p.MinCoverage, e = number(a.MinCoverage, 1); e != nil {
		return p, e
	}
	if p.HealthyMin, e = number(a.Weighted.HealthyMin, .9); e != nil {
		return p, e
	}
	if p.DegradedMin, e = number(a.Weighted.DegradedMin, .5); e != nil {
		return p, e
	}
	if p.DegradedMin >= p.HealthyMin {
		return p, fmt.Errorf("degradedMin must be below healthyMin")
	}
	if p.MaxSkew, e = duration(a.Freshness.MaxSkew, 30*time.Second); e != nil {
		return p, e
	}
	seen := map[string]bool{}
	for _, z := range a.RequiredZones {
		if z == "" || seen[z] {
			return p, fmt.Errorf("requiredZones must be nonempty and unique")
		}
		seen[z] = true
		p.Zones = append(p.Zones, z)
	}
	if len(p.Zones) == 0 {
		return p, fmt.Errorf("research slice requires explicit assessment.requiredZones")
	}
	sort.Strings(p.Zones)
	seen = map[string]bool{}
	var ttl, maxInterval time.Duration
	for i, c := range m.Spec.Checks {
		if !idPattern.MatchString(c.ID) || seen[c.ID] {
			return p, fmt.Errorf("spec.checks[%d].id invalid or duplicated", i)
		}
		seen[c.ID] = true
		x, e := compileProbe(c, p.TargetAddress)
		if e != nil {
			return p, fmt.Errorf("spec.checks[%d] (%s): %w", i, c.ID, e)
		}
		x.ID = c.ID
		x.Weight = 1

		if x.Interval, e = duration(c.Interval, 30*time.Second); e != nil {
			return p, e
		}
		if x.Timeout, e = duration(c.Timeout, 5*time.Second); e != nil {
			return p, e
		}
		if x.Interval < 10*time.Second || x.Interval > 24*time.Hour || x.Timeout < 100*time.Millisecond || x.Timeout > 60*time.Second || x.Timeout >= x.Interval {
			return p, fmt.Errorf("check %s: invalid interval or timeout", c.ID)
		}
		age := max(3*x.Interval, x.Interval+2*x.Timeout)
		x.MaxAge = age
		ttl = max(ttl, age)
		maxInterval = max(maxInterval, x.Interval)
		p.Checks = append(p.Checks, x)
	}
	if len(p.Checks) > 100 {
		return p, fmt.Errorf("research slice supports at most 100 checks per Monitor")
	}
	if len(p.Checks) == 0 {
		return p, fmt.Errorf("checks cannot be empty")
	}
	if p.MaxAge, e = duration(a.Freshness.MaxAge, ttl); e != nil {
		return p, e
	}
	if p.MaxAge < maxInterval {
		return p, fmt.Errorf("maxAge must cover largest interval")
	}
	if a.Freshness.MaxAge != "" {
		for i := range p.Checks {
			p.Checks[i].MaxAge = p.MaxAge
		}
	}
	if a.Checks != nil {
		if len(a.Checks) != len(p.Checks) {
			return p, fmt.Errorf("assessment.checks must cover every check")
		}
		seen = map[string]bool{}
		for _, ac := range a.Checks {
			if seen[ac.ID] {
				return p, fmt.Errorf("duplicate assessment check")
			}
			seen[ac.ID] = true
			found := false
			for j := range p.Checks {
				if p.Checks[j].ID == ac.ID {
					found = true
					p.Checks[j].Required = ac.Required
					p.Checks[j].Weight, e = number(ac.Weight, 1)
					if e != nil {
						return p, e
					}
				}
			}
			if !found {
				return p, fmt.Errorf("unknown assessment check %s", ac.ID)
			}
		}
	}
	for _, rule := range a.Rules {
		priority := 100
		if rule.Priority != nil {
			priority = *rule.Priority
		}
		p.Rules = append(p.Rules, monitor.AssessmentRule{ID: rule.ID, Priority: priority, When: rule.When, State: rule.Then.State, Reason: rule.Then.Reason})
	}
	sort.Slice(p.Rules, func(i, j int) bool {
		if p.Rules[i].Priority == p.Rules[j].Priority {
			return p.Rules[i].ID < p.Rules[j].ID
		}
		return p.Rules[i].Priority > p.Rules[j].Priority
	})
	if _, e := monitor.NewAssessor(p); e != nil {
		return p, e
	}
	var total float64
	for _, c := range p.Checks {
		total += c.Weight
	}
	if total == 0 {
		return p, fmt.Errorf("assessment weights sum to zero")
	}
	for j := range p.Checks {
		p.Checks[j].Weight /= total
	}
	sort.Slice(p.Checks, func(i, j int) bool { return p.Checks[i].ID < p.Checks[j].ID })
	b, e := json.Marshal(p)
	if e != nil {
		return p, e
	}
	if len(b) > (1<<20)-4096 {
		return p, fmt.Errorf("compiled plan exceeds transport frame budget")
	}
	hash := sha256.Sum256(b)
	p.Revision = hex.EncodeToString(hash[:])
	return p, nil
}
