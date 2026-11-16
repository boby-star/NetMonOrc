package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"crypto/sha256"
	"encoding/hex"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

// NormalizeAssessmentPolicy validates the reusable policy independently of a
// particular Monitor. Probe-dependent constraints are checked again on binding.
func NormalizeAssessmentPolicy(data []byte) ([]byte, error) {
	var envelope struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
		Metadata   struct {
			Name string `yaml:"name"`
		} `yaml:"metadata"`
		Spec assessmentSpec `yaml:"spec"`
	}
	if err := decodeResource(data, &envelope); err != nil {
		return nil, err
	}
	if envelope.APIVersion != "netmonorc.io/v1alpha1" || envelope.Kind != "AssessmentPolicy" || !namePattern.MatchString(envelope.Metadata.Name) {
		return nil, fmt.Errorf("invalid AssessmentPolicy envelope")
	}
	a := envelope.Spec
	if a.ZoneAggregation == "" {
		a.ZoneAggregation = "all-required"
	}
	if a.SameZoneAggregation == "" {
		a.SameZoneAggregation = "all"
	}
	if a.DefaultState == "" {
		a.DefaultState = "unknown"
	}
	// Zone aggregation follows the approved table; replica voting stays separate.
	if (a.ZoneAggregation != "all-required" && a.ZoneAggregation != "any") || (a.SameZoneAggregation != "all" && a.SameZoneAggregation != "any" && a.SameZoneAggregation != "quorum") || a.DefaultState != "unknown" {
		return nil, fmt.Errorf("unsupported assessment policy mode")
	}
	if a.Weighted.Enabled != nil && !*a.Weighted.Enabled {
		return nil, fmt.Errorf("weighted.enabled=false is not implemented")
	}
	enabled := true
	a.Weighted.Enabled = &enabled
	coverage, err := number(a.MinCoverage, 1)
	if err != nil {
		return nil, err
	}
	a.MinCoverage = &coverage
	healthy, err := number(a.Weighted.HealthyMin, .9)
	if err != nil {
		return nil, err
	}
	a.Weighted.HealthyMin = &healthy
	degraded, err := number(a.Weighted.DegradedMin, .5)
	if err != nil || degraded >= healthy {
		return nil, fmt.Errorf("invalid weighted thresholds")
	}
	a.Weighted.DegradedMin = &degraded
	skew, err := duration(a.Freshness.MaxSkew, 30*time.Second)
	if err != nil {
		return nil, err
	}
	a.Freshness.MaxSkew = strconv.FormatInt(int64(skew/time.Millisecond), 10) + "ms"
	if a.Freshness.MaxAge != "" {
		age, err := duration(a.Freshness.MaxAge, 0)
		if err != nil {
			return nil, err
		}
		a.Freshness.MaxAge = strconv.FormatInt(int64(age/time.Millisecond), 10) + "ms"
	}
	if a.RequiredZones != nil && len(a.RequiredZones) == 0 {
		return nil, fmt.Errorf("requiredZones must not be an empty list")
	}
	sort.Strings(a.RequiredZones)
	if len(a.Checks) > 100 {
		return nil, fmt.Errorf("at most 100 assessment checks supported")
	}
	for i := range a.Checks {
		weight, err := number(a.Checks[i].Weight, 1)
		if err != nil {
			return nil, err
		}
		a.Checks[i].Weight = &weight
	}
	sort.Slice(a.Checks, func(i, j int) bool { return a.Checks[i].ID < a.Checks[j].ID })
	for i := range a.Rules {
		if a.Rules[i].Priority == nil {
			priority := 100
			a.Rules[i].Priority = &priority
		}
	}
	sort.Slice(a.Rules, func(i, j int) bool {
		if *a.Rules[i].Priority != *a.Rules[j].Priority {
			return *a.Rules[i].Priority > *a.Rules[j].Priority
		}
		return a.Rules[i].ID < a.Rules[j].ID
	})
	normalized, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	// Synthetic checks validate IDs, weights, CEL types and constant bounds only.
	// They are never returned as an executable plan or used as policy defaults.
	validation := a
	if validation.RequiredZones == nil {
		validation.RequiredZones = []string{"validation"}
	}
	ids := []string{"policy_validation"}
	if a.Checks != nil {
		ids = nil
		for _, check := range a.Checks {
			ids = append(ids, check.ID)
		}
	}
	probes := []any{}
	for _, id := range ids {
		probes = append(probes, map[string]any{"id": id, "type": "http", "runner": "native-http", "interval": "10s", "timeout": "1s", "config": map[string]any{"url": "https://192.0.2.1/"}})
	}
	document := map[string]any{"apiVersion": "netmonorc.io/v1alpha1", "kind": "Monitor", "metadata": map[string]string{"name": "policy-validation"}, "spec": map[string]any{"target": map[string]string{"address": "192.0.2.1"}, "checks": probes, "assessment": validation}}
	input, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	if _, err = Compile(input); err != nil {
		return nil, err
	}
	return normalized, nil
}

func ResolveAssessmentReference(p monitor.Plan, ref, hash string) (monitor.Plan, error) {
	if !namePattern.MatchString(ref) || len(hash) != 64 {
		return p, fmt.Errorf("invalid AssessmentPolicy binding")
	}
	p.AssessmentRef = ref
	p.AssessmentHash = hash
	p.Revision = ""
	data, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if len(data) > (1<<20)-4096 {
		return p, fmt.Errorf("resolved plan too large")
	}
	sum := sha256.Sum256(data)
	p.Revision = hex.EncodeToString(sum[:])
	return p, nil
}
