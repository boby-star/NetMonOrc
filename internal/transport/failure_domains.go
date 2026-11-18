package transport

import (
	"fmt"
	"strings"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/secure"
)

// ParseFailureDomains reads Controller-owned topology knowledge. An omitted
// Agent has an unknown domain; neither its zone nor its identity is a substitute.
func ParseFailureDomains(raw string, roster []monitor.Slot) (map[string]string, error) {
	out := map[string]string{}
	if raw == "" {
		return out, nil
	}
	known := map[string]bool{}
	for _, slot := range roster {
		known[slot.Agent] = true
	}
	for _, entry := range strings.Split(raw, ",") {
		agent, domain, ok := strings.Cut(entry, "=")
		if !ok || !known[agent] || !secure.AgentID.MatchString(domain) || out[agent] != "" {
			return nil, fmt.Errorf("failure-domains requires unique roster Agent IDs mapped to domain IDs (lowercase, max 63 characters)")
		}
		out[agent] = domain
	}
	return out, nil
}
