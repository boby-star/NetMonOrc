package resources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

type TargetSpec struct {
	Type        string                    `json:"type"`
	Address     string                    `json:"address,omitempty"`
	NetworkZone string                    `json:"networkZone"`
	Criticality int                       `json:"criticality"`
	Description string                    `json:"description,omitempty"`
	Attributes  map[string]monitor.Scalar `json:"attributes,omitempty"`
}

func (t TargetSpec) Hash() string {
	b, _ := json.Marshal(t)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

var hostnameLabel = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)

func normalizeTargetAddress(value string) (string, error) {
	if ip, err := netip.ParseAddr(value); err == nil {
		if ip.Zone() != "" {
			return "", fmt.Errorf("Target.address must not contain a zone identifier")
		}
		return ip.Unmap().String(), nil
	}
	hostname := strings.TrimSuffix(value, ".")
	if len(hostname) == 0 || len(hostname) > 253 || strings.Trim(hostname, "0123456789.") == "" {
		return "", fmt.Errorf("Target.address requires an IP or hostname")
	}
	for _, label := range strings.Split(hostname, ".") {
		if !hostnameLabel.MatchString(label) {
			return "", fmt.Errorf("Target.address requires an IP or hostname")
		}
	}
	// Preserve the terminal dot: an absolute DNS name must not become a
	// search-domain-dependent name during normalization.
	return strings.ToLower(value), nil
}
func compileTarget(m Manifest) (TargetSpec, error) {
	var wire struct {
		Type        string                    `json:"type"`
		Address     string                    `json:"address"`
		NetworkZone *string                   `json:"networkZone"`
		Criticality *int                      `json:"criticality"`
		Description string                    `json:"description"`
		Attributes  map[string]monitor.Scalar `json:"attributes"`
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(m.Spec, &fields); err != nil || fields == nil {
		return TargetSpec{}, fmt.Errorf("Target.spec requires an object")
	}
	for field, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return TargetSpec{}, fmt.Errorf("Target.spec.%s must not be null", field)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(m.Spec))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return TargetSpec{}, fmt.Errorf("invalid Target.spec: %w", err)
	}
	target := TargetSpec{Type: wire.Type, Address: wire.Address, NetworkZone: "unspecified", Criticality: 3, Description: wire.Description, Attributes: wire.Attributes}
	switch wire.Type {
	case "host", "network-device", "service", "endpoint", "logical":
	default:
		return target, fmt.Errorf("Target.spec.type is required and must be supported")
	}
	if wire.Address == "" && wire.Type != "logical" {
		return target, fmt.Errorf("Target.spec.address is required for non-logical targets")
	}
	if wire.Address != "" {
		address, err := normalizeTargetAddress(wire.Address)
		if err != nil {
			return target, err
		}
		target.Address = address
	}
	if wire.NetworkZone != nil {
		target.NetworkZone = *wire.NetworkZone
	}
	if wire.Criticality != nil {
		target.Criticality = *wire.Criticality
	}
	if target.Criticality < 1 || target.Criticality > 5 || utf8.RuneCountInString(target.Description) > 1024 || len(target.Attributes) > 32 {
		return target, fmt.Errorf("invalid Target criticality, description or attributes limit")
	}
	return target, nil
}

func (r Registry) resolveTarget(m Manifest) (Manifest, string, string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(m.Spec, &fields); err != nil || fields == nil {
		return m, "", "", fmt.Errorf("Monitor.spec requires an object")
	}
	raw, linked := fields["targetRef"]
	if !linked {
		return m, "", "", nil
	}
	if _, inline := fields["target"]; inline {
		return m, "", "", fmt.Errorf("Monitor.spec requires exactly one of target and targetRef")
	}
	var ref string
	if json.Unmarshal(raw, &ref) != nil || !namePattern.MatchString(ref) {
		return m, "", "", fmt.Errorf("Monitor.spec.targetRef requires a resource name")
	}
	target, found := r.Targets[ref]
	if !found {
		return m, "", "", fmt.Errorf("Monitor.spec.targetRef %s is unavailable", ref)
	}
	if target.Address == "" {
		return m, "", "", fmt.Errorf("Monitor.spec.targetRef %s has no probe address", ref)
	}
	delete(fields, "targetRef")
	fields["target"], _ = json.Marshal(map[string]string{"address": target.Address})
	m.Spec, _ = json.Marshal(fields)
	return m, ref, target.Hash(), nil
}
