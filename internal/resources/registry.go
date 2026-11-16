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
	Dependencies     []dependency.Edge
	Placements       map[string]PlacementPolicy
	Roster           []monitor.Slot
	TargetLabels     map[string]map[string]string
	Assessments      map[string]AssessmentPolicy
	CheckProfiles    map[string]CheckProfile
	Active           string
	ExecuteAll       bool
	LoadedRevisions  map[string]string
	AlertChannel     string
	Targets          map[string]TargetSpec
	ManagedWebhooks  bool
	WebhookCIDRs     []netip.Prefix
	Webhooks         map[string]webhook.Endpoint
	Zones            []string
	Diagnostic       *diagnostics.Profile
	Profiles         map[string]diagnostics.Profile
	StaticComparison bool
}

var namePattern = regexp.MustCompile(`^[a-z](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var labelKey = regexp.MustCompile(`^[a-z0-9.-]+$`)

func Decode(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) > 1<<20 {
		return m, fmt.Errorf("resource exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, fmt.Errorf("invalid resource envelope: %w", err)
	}
	if d.Decode(new(yaml.Node)) != io.EOF {
		return m, fmt.Errorf("one resource required")
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
	placed, placement, err := r.resolvePlacement(m)
	if err != nil {
		return monitor.Plan{}, err
	}
	assessmentRegistry := r
	if placement != nil {
		assessmentRegistry.Zones = nil
		for _, slot := range placement.Selected {
			assessmentRegistry.Zones = append(assessmentRegistry.Zones, slot.Zone)
		}
	}
	assessed, assessmentRef, assessmentHash, err := assessmentRegistry.resolveAssessment(placed)
	if err != nil {
		return monitor.Plan{}, err
	}
	expanded, profileBindings, err := r.expandProfiles(assessed)
	if err != nil {
		return monitor.Plan{}, err
	}
	resolved, targetRef, targetHash, err := r.resolveTarget(expanded)
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
	if targetRef != "" {
		target := r.Targets[targetRef]
		labels := map[string]string{}
		for key, value := range r.TargetLabels[targetRef] {
			labels[key] = value
		}
		p, err = config.ResolveTargetReference(p, targetRef, targetHash, monitor.TargetContext{Type: target.Type, NetworkZone: target.NetworkZone, Criticality: target.Criticality, Labels: labels})
		if err != nil {
			return p, err
		}
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
		if placement == nil && !slices.Equal(p.Zones, r.Zones) {
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
		if p.Name == r.Active && placement == nil && !slices.Equal(p.Zones, r.Zones) {
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
