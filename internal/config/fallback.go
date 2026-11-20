package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"gopkg.in/yaml.v3"
)

func compileFallback(n yaml.Node) (*monitor.FallbackPolicy, error) {
	if n.Kind == 0 {
		return nil, nil
	}
	var wire struct {
		Enabled     *bool     `yaml:"enabled"`
		On          *[]string `yaml:"on"`
		Strategy    *string   `yaml:"strategy"`
		MaxAttempts *int      `yaml:"maxAttempts"`
	}
	if err := decodeClosed(n, &wire); err != nil {
		return nil, err
	}
	if wire.Enabled == nil || !*wire.Enabled {
		if wire.On != nil || wire.Strategy != nil || wire.MaxAttempts != nil {
			return nil, fmt.Errorf("disabled fallback forbids other fields")
		}
		return nil, nil
	}
	f := &monitor.FallbackPolicy{Strategy: "same-zone", MaxAttempts: 1}
	if wire.Strategy != nil {
		f.Strategy = *wire.Strategy
	}
	if wire.MaxAttempts != nil {
		f.MaxAttempts = *wire.MaxAttempts
	}
	if wire.On == nil {
		return nil, fmt.Errorf("fallback.on required")
	}
	f.On = append([]string(nil), (*wire.On)...)
	sort.Strings(f.On)
	if err := VerifyFallback(f); err != nil {
		return nil, err
	}
	return f, nil
}
func VerifyFallback(f *monitor.FallbackPolicy) error {
	if f == nil {
		return nil
	}
	if f.MaxAttempts < 1 || f.MaxAttempts > 2 || (f.Strategy != "same-zone" && f.Strategy != "different-zone") || len(f.On) == 0 || len(f.On) > 4 {
		return fmt.Errorf("invalid fallback limits/strategy")
	}
	for i, cause := range f.On {
		if !slices.Contains([]string{"agent_unreachable", "runner_unavailable", "local_execution_error", "config_rejected"}, cause) || i > 0 && cause <= f.On[i-1] {
			return fmt.Errorf("invalid or noncanonical fallback.on")
		}
	}
	return nil
}
func FallbackCheck(c monitor.Check) monitor.Check {
	c.Fallback = nil
	c.Interval = 0
	c.MaxAge = 0
	c.Jitter = 0
	c.Priority = 0
	c.Weight = 0
	c.Required = false
	return c
}
func BindFallbackRoster(p monitor.Plan, roster []monitor.Slot, domains ...map[string]string) (monitor.Plan, error) {
	enabled := false
	for _, c := range p.Checks {
		if c.Fallback != nil {
			enabled = true
			if c.Timeout > 15*time.Second {
				return p, fmt.Errorf("fallback check timeout must fit the 30s Agent lease (<=15s)")
			}
		}
	}
	if !enabled || len(roster) == 0 {
		return p, nil
	}
	if len(domains) > 0 && len(domains[0]) > 0 {
		p.FallbackDomains = map[string]string{}
		for k, v := range domains[0] {
			p.FallbackDomains[k] = v
		}
	}
	p.FallbackRoster = append([]monitor.Slot(nil), roster...)
	sort.Slice(p.FallbackRoster, func(i, j int) bool { return p.FallbackRoster[i].Agent < p.FallbackRoster[j].Agent })
	p.Revision = ""
	b, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if len(b) > (1<<20)-4096 {
		return p, fmt.Errorf("resolved fallback plan too large")
	}
	sum := sha256.Sum256(b)
	p.Revision = hex.EncodeToString(sum[:])
	return p, nil
}
