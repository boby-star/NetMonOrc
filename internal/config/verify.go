package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/boby-star/NetMonOrc/internal/dependency"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

// VerifyPlan checks both the canonical revision and Agent-side resource bounds.
// A valid checksum alone is not permission to execute an arbitrary plan.
func VerifyPlan(p monitor.Plan, zone string, agent ...string) error {
	if p.SameZoneAggregation != "" && p.SameZoneAggregation != "any" && p.SameZoneAggregation != "quorum" {
		return fmt.Errorf("invalid same-zone aggregation")
	}
	revision := p.Revision
	p.Revision = ""
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(b)
	if revision != hex.EncodeToString(sum[:]) || !namePattern.MatchString(p.Name) || (len(p.Checks) == 0 && !p.Disabled) || len(p.Checks) > 100 {
		return fmt.Errorf("invalid plan revision or size")
	}
	if err := dependency.ValidateGraph(p.Dependencies, nil); err != nil {
		return err
	}
	if p.ZoneAggregation != "" && p.ZoneAggregation != "all-required" && p.ZoneAggregation != "any" {
		return fmt.Errorf("invalid zone aggregation")
	}
	if context := p.TargetContext; context != nil {
		switch context.Type {
		case "host", "network-device", "service", "endpoint", "logical":
		default:
			return fmt.Errorf("invalid Target context type")
		}
		if context.Criticality < 1 || context.Criticality > 5 || len(context.NetworkZone) > 63 {
			return fmt.Errorf("invalid Target context binding")
		}
	}
	if (p.TargetRef == "") != (p.TargetHash == "") {
		return fmt.Errorf("incomplete Target binding")
	}
	if p.TargetRef != "" {
		hash, err := hex.DecodeString(p.TargetHash)
		if !namePattern.MatchString(p.TargetRef) || err != nil || len(hash) != 32 {
			return fmt.Errorf("invalid Target binding")
		}
	}
	if (p.AssessmentRef == "") != (p.AssessmentHash == "") {
		return fmt.Errorf("incomplete AssessmentPolicy binding")
	}
	if p.AssessmentRef != "" {
		hash, err := hex.DecodeString(p.AssessmentHash)
		if !namePattern.MatchString(p.AssessmentRef) || err != nil || len(hash) != 32 {
			return fmt.Errorf("invalid AssessmentPolicy binding")
		}
	}
	if len(p.CheckProfiles) > 100 {
		return fmt.Errorf("too many CheckProfile bindings")
	}
	for _, binding := range p.CheckProfiles {
		hash, err := hex.DecodeString(binding.Hash)
		parameters, parameterErr := hex.DecodeString(binding.ParametersHash)
		if !namePattern.MatchString(binding.Ref) || !idPattern.MatchString(binding.As) || err != nil || parameterErr != nil || len(hash) != 32 || len(parameters) != 32 {
			return fmt.Errorf("invalid CheckProfile binding")
		}
	}
	if d := p.Diagnostics; d != nil {
		if d.ComparisonStatic && !d.Enabled {
			return fmt.Errorf("static comparison requires enabled diagnostics")
		}
		if d.Enabled && (d.ProfileRef == "" || len(d.ProfileHash) != 64) {
			return fmt.Errorf("unresolved diagnostic binding")
		}
		if d.Cooldown <= 0 || len(d.OnStates) > 3 || len(d.When) > 2048 {
			return fmt.Errorf("invalid diagnostic binding bounds")
		}
	}
	seenZones := map[string]bool{}
	for _, z := range p.Zones {
		if z == "" || seenZones[z] {
			return fmt.Errorf("invalid zones")
		}
		seenZones[z] = true
	}
	if p.FixedPlacement != nil {
		id := ""
		if len(agent) == 1 {
			id = agent[0]
		}
		if err := verifyFixedPlacement(p.FixedPlacement, id, zone); err != nil {
			return err
		}
	}
	if !seenZones[zone] && p.FixedPlacement == nil && len(p.CheckPlacements) == 0 {
		return fmt.Errorf("assigned zone is not in plan")
	}
	if p.MaxAge <= 0 || p.MaxSkew <= 0 || p.MinCoverage < 0 || p.MinCoverage > 1 || p.DegradedMin < 0 || p.HealthyMin > 1 || p.DegradedMin >= p.HealthyMin {
		return fmt.Errorf("invalid assessment limits")
	}
	seen := map[string]bool{}
	var total float64
	for _, c := range p.Checks {
		if !idPattern.MatchString(c.ID) || seen[c.ID] || (c.Runner != "native-http" && c.Runner != "blackbox") {
			return fmt.Errorf("invalid check identity or runner")
		}
		seen[c.ID] = true
		if c.Priority < 0 || c.Priority > 100 || c.Jitter < 0 || c.Jitter > c.Interval/4 {
			return fmt.Errorf("invalid check priority/jitter")
		}
		if c.Timeout < 100*time.Millisecond || c.Timeout > 60*time.Second || c.Interval < 10*time.Second || c.Interval > 24*time.Hour || c.Timeout >= c.Interval || c.MaxAge < c.Interval {
			return fmt.Errorf("invalid check limits")
		}
		if e := validateProbe(c); e != nil {
			return e
		}

		if math.IsNaN(c.Weight) || math.IsInf(c.Weight, 0) || c.Weight < 0 || c.Weight > 1 {
			return fmt.Errorf("invalid assessment weight")
		}
		total += c.Weight
	}
	if len(p.CheckPlacements) > 100 {
		return fmt.Errorf("too many check placements")
	}
	for id, binding := range p.CheckPlacements {
		if !seen[id] || binding == nil {
			return fmt.Errorf("invalid check placement ID")
		}
		principal := ""
		if len(agent) == 1 {
			principal = agent[0]
		}
		if err := verifyFixedPlacement(binding, principal, zone); err != nil {
			return err
		}
	}
	if total <= 0 && !p.Disabled {
		return fmt.Errorf("zero total weight")
	}
	_, e = monitor.NewAssessor(p)
	return e
}
