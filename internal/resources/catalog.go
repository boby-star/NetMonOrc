package resources

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

const MaxCatalogBytes = 8 << 20
const MaxCatalogResources = 4096

type Compiled struct {
	Revision string
	Monitor  *monitor.Plan
	Profile  *diagnostics.Profile
}

func Key(m Manifest) string      { return m.Kind + "/" + m.Metadata.Name }
func Supported(kind string) bool { return kind == "Monitor" || kind == "DiagnosticProfile" }

// ResolveCatalog validates the proposed complete desired graph, independently
// of document order. It performs no I/O and does not mutate the supplied registry.
func ResolveCatalog(base Registry, current, updates []Manifest) (Registry, map[string]Compiled, error) {
	all := map[string]Manifest{}
	for _, m := range current {
		all[Key(m)] = m
	}
	seen := map[string]bool{}
	for _, m := range updates {
		key := Key(m)
		if seen[key] {
			return base, nil, fmt.Errorf("duplicate resource %s", key)
		}
		seen[key] = true
		all[key] = m
	}
	size := 0
	for _, m := range all {
		data, err := json.Marshal(m)
		if err != nil {
			return base, nil, err
		}
		size += len(data)
	}
	if len(all) > MaxCatalogResources || size > MaxCatalogBytes {
		return base, nil, fmt.Errorf("catalog exceeds research runtime capacity")
	}
	registry := base
	registry.Diagnostic = nil
	registry.Profiles = map[string]diagnostics.Profile{}
	compiled := map[string]Compiled{}
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		m := all[key]
		if err := validateMetadata(m); err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		if m.Kind == "DiagnosticProfile" {
			result, err := registry.CompileResource(m)
			if err != nil {
				return base, nil, fmt.Errorf("%s: %w", key, err)
			}
			registry.Profiles[m.Metadata.Name] = *result.Profile
			compiled[key] = result
		}
	}
	for _, key := range keys {
		m := all[key]
		if m.Kind == "Monitor" {
			result, err := registry.CompileResource(m)
			if err != nil {
				return base, nil, fmt.Errorf("%s: %w", key, err)
			}
			compiled[key] = result
		}
	}
	return registry, compiled, nil
}
func (r Registry) CompileResource(m Manifest) (Compiled, error) {
	if err := validateMetadata(m); err != nil {
		return Compiled{}, err
	}
	if m.Kind == "Monitor" {
		p, err := r.Compile(m)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: p.Revision, Monitor: &p}, nil
	}
	data, err := executionJSON(m)
	if err != nil {
		return Compiled{}, err
	}
	profile, err := config.CompileDiagnostics(data)
	if err != nil {
		return Compiled{}, err
	}
	if err = diagnostics.ValidateLiveProfile(profile); err != nil {
		return Compiled{}, err
	}
	hash, err := config.DiagnosticProfileHash(profile)
	if err != nil {
		return Compiled{}, err
	}
	return Compiled{Revision: hash, Profile: &profile}, nil
}
