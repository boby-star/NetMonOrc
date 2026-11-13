package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

// VerifyPlan checks both the canonical revision and Agent-side resource bounds.
// A valid checksum alone is not permission to execute an arbitrary plan.
func VerifyPlan(p monitor.Plan, zone string) error {
	revision := p.Revision
	p.Revision = ""
	b, e := json.Marshal(p)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(b)
	if revision != hex.EncodeToString(sum[:]) || !namePattern.MatchString(p.Name) || len(p.Checks) == 0 || len(p.Checks) > 100 {
		return fmt.Errorf("invalid plan revision or size")
	}
	seenZones := map[string]bool{}
	for _, z := range p.Zones {
		if z == "" || seenZones[z] {
			return fmt.Errorf("invalid zones")
		}
		seenZones[z] = true
	}
	if !seenZones[zone] {
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
	if total <= 0 {
		return fmt.Errorf("zero total weight")
	}
	_, e = monitor.NewAssessor(p)
	return e
}
