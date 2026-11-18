package transport

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

func TestControllerFailureDomains(t *testing.T) {
	roster := []monitor.Slot{{Agent: "branch", Zone: "office"}, {Agent: "dc", Zone: "server"}}
	for _, raw := range []string{"branch=", "stranger=wan", "branch=wan,branch=wan", "branch=bad domain", "branch=wan,", "branch=wan=other"} {
		if _, err := ParseFailureDomains(raw, roster); err == nil {
			t.Fatalf("invalid trusted domain mapping accepted: %q", raw)
		}
	}
	for _, tc := range []struct {
		raw      string
		distinct int
	}{{"", 0}, {"branch=shared,dc=shared", 1}, {"branch=one,dc=two", 2}} {
		domains, err := ParseFailureDomains(tc.raw, roster)
		if err != nil {
			t.Fatal(err)
		}
		s := admissionServer()
		s.Slots = roster
		s.FailureDomains = domains
		s.active["dc"] = s.active["branch"]
		seen := map[string]bool{}
		for _, executor := range s.Executors(time.Now()) {
			if executor.FailureDomain != domains[executor.ID] {
				t.Fatal("zone or Agent self-report substituted as domain")
			}
			if executor.FailureDomain != "" {
				seen[executor.FailureDomain] = true
			}
		}
		if len(seen) != tc.distinct {
			t.Fatal("incorrect independence count", tc.raw, len(seen))
		}
		for _, row := range s.AgentStatuses(time.Now()) {
			if row.FailureDomain != domains[row.AgentID] {
				t.Fatal("API topology differs from planner")
			}
		}
		delete(s.active, "branch")
		for _, row := range s.AgentStatuses(time.Now()) {
			if row.AgentID == "branch" && row.FailureDomain != domains["branch"] {
				t.Fatal("known topology lost when Agent offline")
			}
		}
	}
}
