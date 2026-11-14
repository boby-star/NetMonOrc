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
	if m.APIVersion != "netmonorc.io/v1alpha1" || m.Kind != "Monitor" || !namePattern.MatchString(m.Metadata.Name) {
		return monitor.Plan{}, fmt.Errorf("this registry supports v1alpha1 Monitor only")
	}
	for key, value := range m.Metadata.Labels {
		if !labelKey.MatchString(key) || utf8.RuneCountInString(key) > 63 || utf8.RuneCountInString(value) > 63 || strings.ToLower(value) != value {
			return monitor.Plan{}, fmt.Errorf("invalid metadata label")
		}
	}
	// Metadata belongs to the resource store; only execution-relevant fields feed IR.
	input := m
	input.Metadata = Metadata{Name: m.Metadata.Name}
	data, err := json.Marshal(input)
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
	if p.Diagnostics != nil && p.Diagnostics.Enabled {
		if r.Diagnostic == nil {
			return p, fmt.Errorf("referenced DiagnosticProfile is unavailable")
		}
		p, err = config.ResolveDiagnostics(p, *r.Diagnostic)
		if err != nil {
			return p, err
		}
	}
	if r.StaticComparison && p.Name == r.Active {
		p, err = config.WithStaticComparison(p)
	}
	return p, err
}
