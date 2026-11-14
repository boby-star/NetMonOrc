package resources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"gopkg.in/yaml.v3"
)

// Registry is a closed catalog. Additional kinds need their own validated compiler.
type Registry struct {
	Active           string
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
	if d.Decode(new(any)) != io.EOF {
		return m, fmt.Errorf("one resource required")
	}
	return m, nil
}
func DecodeYAML(data []byte) (Manifest, error) {
	if len(data) > 1<<20 {
		return Manifest{}, fmt.Errorf("resource exceeds 1 MiB")
	}
	var node any
	d := yaml.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&node); err != nil {
		return Manifest{}, err
	}
	if d.Decode(new(any)) != io.EOF {
		return Manifest{}, fmt.Errorf("one resource required")
	}
	b, err := json.Marshal(node)
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
	data, err := executionJSON(m)
	if err != nil {
		return monitor.Plan{}, err
	}
	p, err := config.Compile(data)
	if err != nil {
		return p, err
	}
	if p.Name == r.Active && !slices.Equal(p.Zones, r.Zones) {
		return p, fmt.Errorf("active Monitor must retain the Controller's fixed zones")
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
	if r.StaticComparison && p.Name == r.Active {
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
