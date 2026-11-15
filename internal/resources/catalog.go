package resources

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/webhook"
)

const MaxCatalogBytes = 8 << 20
const MaxCatalogResources = 4096

type Compiled struct {
	Revision string
	Monitor  *monitor.Plan
	Profile  *diagnostics.Profile
	Rule     *alerting.Rule
	Endpoint *webhook.Endpoint
	Target   *TargetSpec
}

func Key(m Manifest) string { return m.Kind + "/" + m.Metadata.Name }
func Supported(kind string) bool {
	return kind == "Monitor" || kind == "DiagnosticProfile" || kind == "AlertRule" || kind == "WebhookEndpoint" || kind == "Target"
}

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
	registry.Targets = map[string]TargetSpec{}
	registry.Webhooks = map[string]webhook.Endpoint{}
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
		if m.Kind != "Target" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		registry.Targets[m.Metadata.Name] = *result.Target
		compiled[key] = result
	}
	activeEndpoints := 0
	for _, key := range keys {
		m := all[key]
		if m.Kind != "WebhookEndpoint" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		if result.Endpoint.Enabled {
			activeEndpoints++
		}
		if activeEndpoints > 1 {
			return base, nil, fmt.Errorf("at most one enabled WebhookEndpoint is allowed")
		}
		registry.Webhooks[m.Metadata.Name] = *result.Endpoint
		compiled[key] = result
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
	for _, key := range keys {
		m := all[key]
		if m.Kind != "AlertRule" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		if _, found := compiled["Monitor/"+result.Rule.Monitor]; !found {
			return base, nil, fmt.Errorf("%s: monitorRef is unavailable", key)
		}
		if registry.ManagedWebhooks {
			if _, found := registry.Webhooks[result.Rule.Channel]; !found && result.Rule.Enabled {
				return base, nil, fmt.Errorf("%s: channelRef is unavailable", key)
			}
		} else if registry.AlertChannel == "" || result.Rule.Channel != registry.AlertChannel {
			return base, nil, fmt.Errorf("%s: channelRef is not the configured webhook", key)
		}
		compiled[key] = result
	}
	return registry, compiled, nil
}
func (r Registry) CompileResource(m Manifest) (Compiled, error) {
	if err := validateMetadata(m); err != nil {
		return Compiled{}, err
	}
	if m.Kind == "Target" {
		target, err := compileTarget(m)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: target.Hash(), Target: &target}, nil
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
	if m.Kind == "WebhookEndpoint" {
		endpoint, err := config.CompileWebhook(data)
		if err != nil {
			return Compiled{}, err
		}
		if err = webhook.Allowed(endpoint.URL, r.WebhookCIDRs); err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: endpoint.Hash(), Endpoint: &endpoint}, nil
	}
	if m.Kind == "AlertRule" {
		rule, err := config.CompileAlert(data)
		if err != nil {
			return Compiled{}, err
		}
		evaluator, err := alerting.New(rule)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: evaluator.Hash, Rule: &rule}, nil
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
