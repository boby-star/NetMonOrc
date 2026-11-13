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
	b, e := json.Marshal(profile)
	if e != nil {
		return p, e
	}
	h := sha256.Sum256(b)
	binding := *p.Diagnostics
	binding.ProfileHash = hex.EncodeToString(h[:])
	p.Diagnostics = &binding
	p.Revision = ""
	b, e = json.Marshal(p)
	if e != nil {
		return p, e
	}
	if len(b) > (1<<20)-4096 {
		return p, fmt.Errorf("resolved plan too large")
	}
	h = sha256.Sum256(b)
	p.Revision = hex.EncodeToString(h[:])
	return p, nil
}

func VerifyDiagnosticCheck(c monitor.Check) error {
	if (c.Runner != "blackbox" && c.Runner != "native-http") || !idPattern.MatchString(c.ID) || c.Timeout < 100*time.Millisecond || c.Timeout > 60*time.Second {
		return fmt.Errorf("invalid diagnostic check identity/timeout")
	}
	return validateProbe(c)
}
