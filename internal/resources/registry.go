package resources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/webhook"
	"io"
	"net/netip"
	"regexp"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/dependency"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"gopkg.in/yaml.v3"
)

// Registry is a closed catalog. Additional kinds need their own validated compiler.
type Registry struct {
	Components         map[string]Component
	AgentLabels        map[string]map[string]string
	FailureDomains     map[string]string
	Dependencies       []dependency.Edge
	Placements         map[string]PlacementPolicy
	Roster             []monitor.Slot
	TargetLabels       map[string]map[string]string
	Assessments        map[string]AssessmentPolicy
	CheckProfiles      map[string]CheckProfile
	Active             string
	ExecuteAll         bool
	LoadedRevisions    map[string]string
	ResolvedPlacements map[string]monitor.ResolvedPlacement
	AlertChannel       string
	Targets            map[string]TargetSpec
	ManagedWebhooks    bool
	WebhookCIDRs       []netip.Prefix
	Webhooks           map[string]webhook.Endpoint
	Zones              []string
	Diagnostic         *diagnostics.Profile
	Profiles           map[string]diagnostics.Profile
	StaticComparison   bool
}

var namePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var labelKey = regexp.MustCompile(`^[a-z0-9.-]+$`)

func Decode(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) > 1<<20 {
		return m, fmt.Errorf("resource exceeds 1 MiB")
	}
	if err := DecodeJSON(data, &m); err != nil {
		return m, fmt.Errorf("invalid resource envelope: %w", err)
	}
	return m, nil
}
func DecodeYAML(data []byte) (Manifest, error) {
	if len(data) > 1<<20 {
		return Manifest{}, fmt.Errorf("resource exceeds 1 MiB")
	}
	var node yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&node); err != nil {
		return Manifest{}, err
	}
	if d.Decode(new(yaml.Node)) != io.EOF {
		return Manifest{}, fmt.Errorf("one resource required")
	}
	b, err := YAMLJSON(&node)
	if err != nil {
		return Manifest{}, fmt.Errorf("JSON-compatible resource required")
	}
	return Decode(b)
}
func (r Registry) Compile(m Manifest) (monitor.Plan, error) {
	if m.Kind != "Monitor" {
		return monitor.Plan{}, fmt.Errorf("Monitor required")
	}
	if err := validateMetadata(m); err != nil {
		return monitor.Plan{}, err
	}
	var err error
	r, m, err = r.lowerMonitorDependencies(m)
	if err != nil {
		return monitor.Plan{}, err
	}
	placed, placement, err := r.resolvePlacement(m)
	if err != nil {
		return monitor.Plan{}, err
	}
	expanded, profileBindings, err := r.expandProfiles(placed)
	if err != nil {
		return monitor.Plan{}, err
	}
	placed, checkPlacements, err := r.resolveCheckPlacements(expanded)
	if err != nil {
		return monitor.Plan{}, err
	}
	assessmentRegistry := r
	if placement != nil && len(placement.Selected) > 0 {
		assessmentRegistry.Zones = nil
		for _, slot := range placement.Selected {
			if !slices.Contains(assessmentRegistry.Zones, slot.Zone) {
				assessmentRegistry.Zones = append(assessmentRegistry.Zones, slot.Zone)
			}
		}
	}
	if placement != nil && placement.Strategy != "" && len(placement.Selected) == 0 && len(placement.Candidates) > 0 {
		assessmentRegistry.Zones = nil
		for _, slot := range placement.Candidates {
			if !slices.Contains(assessmentRegistry.Zones, slot.Zone) {
				assessmentRegistry.Zones = append(assessmentRegistry.Zones, slot.Zone)
			}
		}
	}
	if len(checkPlacements) > 0 {
		zones := map[string]bool{}
		var body struct {
			Checks []struct {
				ID      string `json:"id"`
				Enabled *bool  `json:"enabled"`
			} `json:"checks"`
		}
		_ = json.Unmarshal(placed.Spec, &body)
		for _, c := range body.Checks {
			if c.Enabled != nil && !*c.Enabled {
				continue
			}
			if binding := checkPlacements[c.ID]; binding != nil {
				for _, z := range binding.ScopeZones {
					zones[z] = true
				}
			} else {
				for _, z := range assessmentRegistry.Zones {
					zones[z] = true
				}
			}
		}
		assessmentRegistry.Zones = nil
		for z := range zones {
			assessmentRegistry.Zones = append(assessmentRegistry.Zones, z)
		}
		sort.Strings(assessmentRegistry.Zones)
	}
	assessed, assessmentRef, assessmentHash, err := assessmentRegistry.resolveAssessment(placed)
	if err != nil {
		return monitor.Plan{}, err
	}
	resolved, targetRef, targetHash, err := r.resolveTarget(assessed)
	if err != nil {
		return monitor.Plan{}, err
	}
	data, err := executionJSON(resolved)
	if err != nil {
		return monitor.Plan{}, err
	}
	p, err := config.Compile(data)
	if err != nil {
		return p, err
	}
	for _, rule := range p.Responses {
		if len(r.Roster) == 0 {
			continue
		}
		found := false
		for _, slot := range r.Roster {
			found = found || slot.Agent == rule.AgentRef
		}
		if !found {
			return p, fmt.Errorf("response Agent %s is outside Controller roster", rule.AgentRef)
		}
	}
	p.CheckPlacements = checkPlacements
	if len(checkPlacements) > 0 {
		p, err = config.ResolveFixedPlacement(p, placement)
		if err != nil {
			return p, err
		}
	}
	if targetRef != "" {
		target := r.Targets[targetRef]
		labels := map[string]string{}
		for key, value := range r.TargetLabels[targetRef] {
			labels[key] = value
		}
		p, err = config.ResolveTargetReference(p, targetRef, targetHash, monitor.TargetContext{Type: target.Type, NetworkZone: target.NetworkZone, Criticality: target.Criticality, Labels: labels, Attributes: target.Attributes})
		if err != nil {
			return p, err
		}
	}
	p, err = config.ResolveMonitorLabels(p, m.Metadata.Labels)
	if err != nil {
		return p, err
	}
	roots := []dependency.Reference{{Kind: "Monitor", Name: p.Name}}
	if p.TargetRef != "" {
		roots = append(roots, dependency.Reference{Kind: "Target", Name: p.TargetRef})
	}
	p, err = config.ResolveDependencyContext(p, dependency.Context(r.Dependencies, roots...))
	if err != nil {
		return p, err
	}
	if len(profileBindings) > 0 {
		p, err = config.ResolveCheckProfiles(p, profileBindings)
		if err != nil {
			return p, err
		}
	}
	if assessmentRef != "" {
		p, err = config.ResolveAssessmentReference(p, assessmentRef, assessmentHash)
		if err != nil {
			return p, err
		}
	}
	if placement != nil {
		p, err = config.ResolveFixedPlacement(p, placement)
		if err != nil {
			return p, err
		}
	}
	if r.ExecuteAll {
		if placement == nil && len(checkPlacements) == 0 && !slices.Equal(p.Zones, r.Zones) {
			return p, fmt.Errorf("Monitor without placement must retain Controller fixed zones")
		}
		for _, zone := range p.Zones {
			if !slices.Contains(r.Zones, zone) {
				return p, fmt.Errorf("assessment zone is outside Controller roster")
			}
		}
	} else {
		if p.Name == r.Active && placement != nil {
			for _, zone := range p.Zones {
				if !slices.Contains(r.Zones, zone) {
					return p, fmt.Errorf("assessment zone is outside Controller roster")
				}
			}
		}
		if p.Name == r.Active && placement == nil && len(checkPlacements) == 0 && !slices.Equal(p.Zones, r.Zones) {
			return p, fmt.Errorf("active Monitor must retain the Controller's fixed zones")
		}
	}
	if p.Diagnostics != nil && p.Diagnostics.ProfileRef != "" {
		profile, found := r.Profiles[p.Diagnostics.ProfileRef]
		if !found && r.Diagnostic != nil && r.Diagnostic.Name == p.Diagnostics.ProfileRef {
			profile = *r.Diagnostic
			found = true
		}
		if !found && p.Diagnostics.Enabled {
			return p, fmt.Errorf("referenced DiagnosticProfile %s is unavailable", p.Diagnostics.ProfileRef)
		}
		if found {
			p, err = config.ResolveDiagnosticReference(p, profile)
			if err != nil {
				return p, err
			}
		}

	}
	if r.StaticComparison && (r.ExecuteAll || p.Name == r.Active) {
		p, err = config.WithStaticComparison(p)
	}
	if err == nil {
		p, err = config.BindFallbackRoster(p, r.Roster, r.FailureDomains)
	}
	if err == nil {
		p, err = r.bindComponents(p)
	}
	if err == nil && p.HasDynamicPlacement() {
		bindings := map[string]*monitor.FixedPlacement{m.Metadata.Name: p.FixedPlacement}
		for id, binding := range p.CheckPlacements {
			bindings[m.Metadata.Name+"/"+id] = binding
		}
		for key, binding := range bindings {
			if binding == nil || binding.Strategy == "" {
				continue
			}
			if prior, exists := r.ResolvedPlacements[key]; exists && len(prior.Selected) > 0 {
				template, e := r.PlacementTemplate(m)
				if e != nil {
					return p, e
				}
				if prior.TemplateHash != template.Revision {
					return template, nil
				}
			}
		}
	}
	if err == nil && len(p.Rules) > 0 {
		assessor, checkErr := monitor.NewAssessor(p)
		if checkErr != nil {
			return p, checkErr
		}
		sources := p.ReplicaSources()
		if p.FixedPlacement == nil {
			sources = r.Roster
		}
		if checkErr = assessor.ValidateSources(sources); checkErr != nil {
			return p, checkErr
		}
	}
	return p, err
}

func validateMetadata(m Manifest) error {
	if m.APIVersion != "netmonorc.io/v1alpha1" || !Supported(m.Kind) || !namePattern.MatchString(m.Metadata.Name) {
		return fmt.Errorf("supported v1alpha1 kind and resource name required")
	}
	for key, value := range m.Metadata.Labels {
		if !labelKey.MatchString(key) || utf8.RuneCountInString(key) > 63 || utf8.RuneCountInString(value) > 63 || strings.ToLower(value) != value {
			return fmt.Errorf("invalid metadata label")
		}
	}
	return nil
}
func executionJSON(m Manifest) ([]byte, error) {
	m.Metadata = Metadata{Name: m.Metadata.Name}
	return json.Marshal(m)
}

// PlacementTemplate binds cached selections to every expanded dependency, not
// only the policy. A target/runner change must first withdraw old execution.
func (r Registry) PlacementTemplate(m Manifest) (monitor.Plan, error) {
	copied := make(map[string]monitor.ResolvedPlacement, len(r.ResolvedPlacements))
	for name, value := range r.ResolvedPlacements {
		if name != m.Metadata.Name && !strings.HasPrefix(name, m.Metadata.Name+"/") {
			copied[name] = value
		}
	}
	r.ResolvedPlacements = copied
	return r.Compile(m)
}
