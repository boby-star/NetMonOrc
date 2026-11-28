package config

import (
	"fmt"
	"net/netip"
	"sort"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

type responseSpec struct {
	ID        string `yaml:"id"`
	When      string `yaml:"when"`
	BlockPort struct {
		AgentRef   string `yaml:"agentRef"`
		Protocol   string `yaml:"protocol"`
		Port       int    `yaml:"port"`
		SourceCIDR string `yaml:"sourceCIDR"`
		Duration   string `yaml:"duration"`
	} `yaml:"blockPort"`
}

func compileResponses(p *monitor.Plan, specs []responseSpec) error {
	for _, s := range specs {
		if s.BlockPort.Protocol != "tcp" {
			return fmt.Errorf("responses support explicit tcp protocol only")
		}
		d, err := duration(s.BlockPort.Duration, 0)
		if err != nil {
			return err
		}
		p.Responses = append(p.Responses, monitor.ResponseRule{ID: s.ID, When: s.When, AgentRef: s.BlockPort.AgentRef, Port: s.BlockPort.Port, SourceCIDR: s.BlockPort.SourceCIDR, Duration: d})
	}
	sort.Slice(p.Responses, func(i, j int) bool { return p.Responses[i].ID < p.Responses[j].ID })
	return VerifyResponses(p.Responses)
}

func VerifyResponses(rules []monitor.ResponseRule) error {
	if len(rules) > 8 {
		return fmt.Errorf("at most 8 response rules supported")
	}
	seen := map[string]bool{}
	for _, r := range rules {
		prefix, err := netip.ParsePrefix(r.SourceCIDR)
		if !idPattern.MatchString(r.ID) || seen[r.ID] || !namePattern.MatchString(r.AgentRef) || r.Port < 1024 || r.Port > 65535 || r.Port == 5432 || r.Port == 8443 || r.Port == 9443 || r.Duration < 5*time.Second || r.Duration > 5*time.Minute || err != nil || prefix.Bits() != prefix.Addr().BitLen() || !prefix.Addr().IsGlobalUnicast() || prefix.Addr().IsLoopback() || prefix.Addr().Is4In6() {
			return fmt.Errorf("invalid bounded response rule %q", r.ID)
		}
		if _, err := diagnostics.CompileTrigger(r.When); err != nil {
			return fmt.Errorf("response %s: %w", r.ID, err)
		}
		seen[r.ID] = true
	}
	return nil
}
