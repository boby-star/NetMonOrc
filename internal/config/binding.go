package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

type diagnosticBindingSpec struct {
	Enabled    bool     `yaml:"enabled"`
	ProfileRef string   `yaml:"profileRef"`
	OnStates   []string `yaml:"onStates"`
	When       string   `yaml:"when"`
	Cooldown   string   `yaml:"cooldown"`
}

func compileBinding(s diagnosticBindingSpec) (*monitor.DiagnosticBinding, error) {
	b := &monitor.DiagnosticBinding{Enabled: s.Enabled, ProfileRef: s.ProfileRef, OnStates: s.OnStates, When: s.When}
	if b.Enabled && !namePattern.MatchString(b.ProfileRef) {
		return nil, fmt.Errorf("enabled diagnostics require profileRef")
	}
	if b.ProfileRef != "" && !namePattern.MatchString(b.ProfileRef) {
		return nil, fmt.Errorf("invalid diagnostic profileRef")
	}
	if b.OnStates == nil {
		b.OnStates = []string{"degraded", "down"}
	}
	seen := map[string]bool{}
	for _, s := range b.OnStates {
		if seen[s] || (s != "degraded" && s != "down" && s != "unknown") {
			return nil, fmt.Errorf("invalid diagnostic onStates")
		}
		seen[s] = true
	}
	sort.Strings(b.OnStates)
	if b.When == "" {
		b.When = "true"
	}
	if _, e := diagnostics.CompileTrigger(b.When); e != nil {
		return nil, e
	}
	var e error
	b.Cooldown, e = duration(s.Cooldown, 5*time.Minute)
	return b, e
}

// ResolveDiagnostics expands the referenced profile into the effective hash.
// Profile changes therefore invalidate regular observations as well.
func ResolveDiagnostics(p monitor.Plan, profile diagnostics.Profile) (monitor.Plan, error) {
	if p.Diagnostics == nil || !p.Diagnostics.Enabled || p.Diagnostics.ProfileRef != profile.Name {
		return p, fmt.Errorf("diagnostic binding/profile mismatch")
	}
	return ResolveDiagnosticReference(p, profile)
}

// ResolveDiagnosticReference includes an explicit profile dependency in the
// effective revision even when its execution binding is disabled.
func ResolveDiagnosticReference(p monitor.Plan, profile diagnostics.Profile) (monitor.Plan, error) {
	if err := ValidateDiagnosticMonitor(p, profile); err != nil {
		return p, err
	}
	if p.Diagnostics == nil || p.Diagnostics.ProfileRef != profile.Name {
		return p, fmt.Errorf("diagnostic binding/profile mismatch")
	}
	hash, e := DiagnosticProfileHash(profile)
	if e != nil {
		return p, e
	}
	binding := *p.Diagnostics
	binding.ProfileHash = hash
	p.Diagnostics = &binding
	p.Revision = ""
	b, e := json.Marshal(p)
	if e != nil {
		return p, e
	}
	if len(b) > (1<<20)-4096 {
		return p, fmt.Errorf("resolved plan too large")
	}
	h := sha256.Sum256(b)
	p.Revision = hex.EncodeToString(h[:])
	return p, nil
}

func VerifyDiagnosticCheck(c monitor.Check) error {
	if c.Fallback != nil {
		return fmt.Errorf("one-shot check cannot contain fallback")
	}
	if (c.Runner != "blackbox" && c.Runner != "native-http") || !idPattern.MatchString(c.ID) || c.Timeout < 100*time.Millisecond || c.Timeout > 60*time.Second {
		return fmt.Errorf("invalid diagnostic check identity/timeout")
	}
	return validateProbe(c)
}

// WithStaticComparison selects the research baseline without adding a DSL enum.
// Include the selection in the effective revision to keep experiments distinct.
func WithStaticComparison(p monitor.Plan) (monitor.Plan, error) {
	if p.Diagnostics == nil || !p.Diagnostics.Enabled || p.Diagnostics.ProfileHash == "" {
		return p, fmt.Errorf("static comparison requires resolved enabled diagnostics")
	}
	b := *p.Diagnostics
	b.ComparisonStatic = true
	p.Diagnostics = &b
	p.Revision = ""
	data, e := json.Marshal(p)
	if e != nil {
		return p, e
	}
	if len(data) > (1<<20)-4096 {
		return p, fmt.Errorf("compiled comparison plan exceeds frame budget")
	}
	hash := sha256.Sum256(data)
	p.Revision = hex.EncodeToString(hash[:])
	return p, nil
}

func DiagnosticProfileHash(profile diagnostics.Profile) (string, error) {
	profile = CanonicalDiagnosticProfile(profile)
	data, err := json.Marshal(profile)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// CanonicalDiagnosticProfile copies unordered catalog collections before sorting.
// Fixed placement AgentRefs retain their order: it is executor preference.
func CanonicalDiagnosticProfile(p diagnostics.Profile) diagnostics.Profile {
	p.Hypotheses = append([]string(nil), p.Hypotheses...)
	sort.Strings(p.Hypotheses)
	p.Actions = append([]diagnostics.Action(nil), p.Actions...)
	for i := range p.Actions {
		p.Actions[i].Placement.Zones = append([]string(nil), p.Actions[i].Placement.Zones...)
		sort.Strings(p.Actions[i].Placement.Zones)
	}
	sort.Slice(p.Actions, func(i, j int) bool { return p.Actions[i].ID < p.Actions[j].ID })
	p.Steps = append([]diagnostics.Step(nil), p.Steps...)
	for i := range p.Steps {
		p.Steps[i].DependsOn = append([]string(nil), p.Steps[i].DependsOn...)
		sort.Strings(p.Steps[i].DependsOn)
	}
	sort.Slice(p.Steps, func(i, j int) bool { return p.Steps[i].ID < p.Steps[j].ID })
	if p.Confirmation != nil {
		c := *p.Confirmation
		c.ActionRefs = append([]string(nil), c.ActionRefs...)
		sort.Strings(c.ActionRefs)
		p.Confirmation = &c
	}
	return p
}

// ResolveTargetReference makes changes to the normalized reusable target part
// of the effective revision, without merging its labels or zone into Monitor.
func ResolveTargetReference(p monitor.Plan, ref, hash string, context ...monitor.TargetContext) (monitor.Plan, error) {
	if !namePattern.MatchString(ref) || len(hash) != 64 {
		return p, fmt.Errorf("invalid resolved Target identity")
	}
	p.TargetRef = ref
	if len(context) > 0 {
		snapshot := context[0]
		p.TargetContext = &snapshot
	}
	p.TargetHash = hash
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

// ResolveCheckProfiles includes template and bound-parameter provenance even
// when an updated parameter does not change a particular generated field.
func ResolveCheckProfiles(p monitor.Plan, bindings []monitor.CheckProfileBinding) (monitor.Plan, error) {
	p.CheckProfiles = append([]monitor.CheckProfileBinding(nil), bindings...)
	sort.Slice(p.CheckProfiles, func(i, j int) bool {
		a, b := p.CheckProfiles[i], p.CheckProfiles[j]
		if a.As != b.As {
			return a.As < b.As
		}
		return a.Ref < b.Ref
	})
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
