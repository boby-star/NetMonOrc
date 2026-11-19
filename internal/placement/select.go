// Package placement chooses complete regular assignment sets from trusted inputs.
package placement

import (
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/probe"
)

// Select retains a feasible assignment. Only a new assignment advances the
// policy cursor; a heartbeat is neither a rebalance request nor a new probe.
func Select(plan monitor.Plan, agents []agentstatus.View, cursor int64, now time.Time) ([]monitor.Slot, int64) {
	policy := plan.FixedPlacement
	if policy == nil || policy.Strategy == "" {
		return nil, cursor
	}
	views := map[string]agentstatus.View{}
	for _, a := range agents {
		views[a.AgentID] = a
	}
	var eligible []monitor.Slot
	for _, slot := range policy.Candidates {
		a, ok := views[slot.Agent]
		if !ok || a.NetworkZone != slot.Zone || !eligibleFor(plan, a, now) || (policy.DistinctBy == "failureDomain" && policy.FailureDomains[slot.Agent] == "") {
			continue
		}
		eligible = append(eligible, slot)
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i].Agent < eligible[j].Agent })
	if len(policy.Selected) == policy.Replicas {
		valid := true
		seen := map[string]bool{}
		for _, slot := range policy.Selected {
			key := distinctValue(policy, slot)
			valid = valid && slices.Contains(eligible, slot) && key != "" && !seen[key]
			seen[key] = true
		}
		if valid {
			return append([]monitor.Slot(nil), policy.Selected...), cursor
		}
	}
	if len(eligible) < policy.Replicas {
		return []monitor.Slot{}, cursor
	}
	if policy.Strategy == "leastLoaded" {
		sort.SliceStable(eligible, func(i, j int) bool {
			left, right := views[eligible[i].Agent], views[eligible[j].Agent]
			return int64(*left.ActiveProbes)*int64(*right.MaxConcurrentProbes) < int64(*right.ActiveProbes)*int64(*left.MaxConcurrentProbes)
		})
		var selected []monitor.Slot
		used := map[string]bool{}
		for _, slot := range eligible {
			key := distinctValue(policy, slot)
			if key == "" || used[key] {
				continue
			}
			selected = append(selected, slot)
			used[key] = true
			if len(selected) == policy.Replicas {
				return selected, cursor
			}
		}
		return []monitor.Slot{}, cursor
	}
	if cursor < 0 {
		cursor = 0
	}
	selected := make([]monitor.Slot, 0, policy.Replicas)
	used := map[string]bool{}
	for i := 0; i < len(eligible); i++ {
		slot := eligible[(int(cursor%int64(len(eligible)))+i)%len(eligible)]
		key := distinctValue(policy, slot)
		if key == "" || used[key] {
			continue
		}
		selected = append(selected, slot)
		used[key] = true
		if len(selected) == policy.Replicas {
			return selected, (cursor + int64(i+1)) % int64(len(eligible))
		}
	}
	return []monitor.Slot{}, cursor
}
func eligibleFor(plan monitor.Plan, a agentstatus.View, now time.Time) bool {
	if a.OnlineState != "online" || a.LastHeartbeat == nil || now.Sub(*a.LastHeartbeat) > 15*time.Second || a.LastHeartbeat.After(now.Add(5*time.Second)) || a.CertExpiresAt == nil || !now.Before(*a.CertExpiresAt) || a.MaxConcurrentProbes == nil || a.ActiveProbes == nil || *a.MaxConcurrentProbes <= 0 || *a.MaxConcurrentProbes > 1024 || *a.ActiveProbes < 0 || *a.ActiveProbes > *a.MaxConcurrentProbes {
		return false
	}
	var allowed []netip.Prefix
	for _, capability := range a.Capabilities {
		if cidr, ok := strings.CutPrefix(capability, "target-cidr:"); ok {
			prefix, err := netip.ParsePrefix(cidr)
			if err == nil {
				allowed = append(allowed, prefix)
			}
		}
	}
	for _, check := range plan.Checks {
		if !slices.Contains(a.Capabilities, check.Runner) || !slices.Contains(a.Capabilities, check.Type) || (len(check.Extract) > 0 && !slices.Contains(a.Capabilities, "extraction:regex")) || probe.CheckTargetPolicy(plan, check, allowed) != nil {
			return false
		}
	}
	return slices.Contains(a.Capabilities, "placement:dynamic")
}

func distinctValue(p *monitor.FixedPlacement, s monitor.Slot) string {
	switch p.DistinctBy {
	case "zone":
		return s.Zone
	case "failureDomain":
		return p.FailureDomains[s.Agent]
	default:
		return s.Agent
	}
}
