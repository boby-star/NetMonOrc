package resources

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/dependency"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"github.com/boby-star/NetMonOrc/internal/webhook"
)

const MaxCatalogBytes = 8 << 20
const MaxCatalogResources = 4096

type Compiled struct {
	Component    *Component
	Dependency   *dependency.Edge
	Group        *resourcegroup.Spec
	GroupMembers []string
	Revision     string
	Monitor      *monitor.Plan
	Profile      *diagnostics.Profile
	Rule         *alerting.Rule
	Endpoint     *webhook.Endpoint
	Target       *TargetSpec
	CheckProfile *CheckProfile
	Assessment   *AssessmentPolicy
	Placement    *PlacementPolicy
}

func Key(m Manifest) string { return m.Kind + "/" + m.Metadata.Name }
func Supported(kind string) bool {
	return kind == "Component" || kind == "Dependency" || kind == "ResourceGroup" || kind == "Monitor" || kind == "DiagnosticProfile" || kind == "AlertRule" || kind == "WebhookEndpoint" || kind == "Target" || kind == "CheckProfile" || kind == "AssessmentPolicy" || kind == "PlacementPolicy"
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
	registry.Components = map[string]Component{}
	registry.Placements = map[string]PlacementPolicy{}
	registry.Assessments = map[string]AssessmentPolicy{}
	registry.CheckProfiles = map[string]CheckProfile{}
	registry.Targets = map[string]TargetSpec{}
	registry.TargetLabels = map[string]map[string]string{}
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
		registry.TargetLabels[m.Metadata.Name] = m.Metadata.Labels
		compiled[key] = result
	}
	for _, key := range keys {
		m := all[key]
		if m.Kind != "CheckProfile" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		registry.CheckProfiles[m.Metadata.Name] = *result.CheckProfile
		compiled[key] = result
	}
	for _, key := range keys {
		m := all[key]
		if m.Kind != "AssessmentPolicy" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		registry.Assessments[m.Metadata.Name] = *result.Assessment
		compiled[key] = result
	}
	for _, key := range keys {
		m := all[key]
		if m.Kind != "PlacementPolicy" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		registry.Placements[m.Metadata.Name] = *result.Placement
		compiled[key] = result
	}
	for _, key := range keys {
		m := all[key]
		if m.Kind != "Component" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		registry.Components[m.Metadata.Name] = *result.Component
		compiled[key] = result
	}
	if _, err := registry.bindComponents(monitor.Plan{}); err != nil {
		return base, nil, err
	}
	registry.Dependencies = nil
	known := map[string]bool{}
	for key := range all {
		known[key] = true
	}
	for _, key := range keys {
		m := all[key]
		if m.Kind != "Dependency" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		registry.Dependencies = append(registry.Dependencies, *result.Dependency)
		compiled[key] = result
	}
	// Expand all Monitor shorthand before validating the combined requires DAG.
	for _, key := range keys {
		m := all[key]
		if m.Kind != "Monitor" {
			continue
		}
		_, refs, err := monitorDependencyRefs(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		registry.Dependencies, err = derivedRequirements(m, refs, registry.Dependencies, registry.Targets)
		if err != nil {
			return base, nil, err
		}
	}
	if err := dependency.ValidateGraph(registry.Dependencies, known); err != nil {
		return base, nil, err
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
	for _, key := range keys {
		m := all[key]
		if m.Kind != "ResourceGroup" {
			continue
		}
		result, err := registry.CompileResource(m)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		ids, rev, err := bindGroup(*result.Group, all, compiled)
		if err != nil {
			return base, nil, fmt.Errorf("%s: %w", key, err)
		}
		result.GroupMembers = ids
		result.Revision = rev
		compiled[key] = result
	}
	return registry, compiled, nil
}
func (r Registry) CompileResource(m Manifest) (Compiled, error) {
	if err := validateMetadata(m); err != nil {
		return Compiled{}, err
	}
	if m.Kind == "Dependency" {
		edge, err := compileDependency(m)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Dependency: &edge, Revision: valueHash(edge.Spec)}, nil
	}
	if m.Kind == "ResourceGroup" {
		g, err := compileGroup(m.Spec)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: valueHash(g), Group: &g}, nil
	}
	if m.Kind == "PlacementPolicy" {
		placement, err := compilePlacement(m.Spec)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: placement.Hash(), Placement: &placement}, nil
	}
	if m.Kind == "AssessmentPolicy" {
		assessment, err := compileAssessmentPolicy(m)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: assessment.Hash(), Assessment: &assessment}, nil
	}
	if m.Kind == "Component" {
		c, err := compileComponent(m)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: c.Hash(), Component: &c}, nil
	}
	if m.Kind == "CheckProfile" {
		profile, err := compileCheckProfile(m)
		if err != nil {
			return Compiled{}, err
		}
		return Compiled{Revision: profile.Hash(), CheckProfile: &profile}, nil
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
