package resources

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"gopkg.in/yaml.v3"
)

type ProfileParameter struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Required    bool            `json:"required,omitempty"`
	Default     json.RawMessage `json:"default,omitempty"`
	Description string          `json:"description,omitempty"`
}
type CheckProfile struct {
	Parameters  []ProfileParameter `json:"parameters,omitempty"`
	Checks      []json.RawMessage  `json:"checks"`
	Description string             `json:"description,omitempty"`
}
type profileUsage struct {
	Ref        string                     `json:"ref"`
	As         string                     `json:"as"`
	Parameters map[string]json.RawMessage `json:"parameters,omitempty"`
}

var checkIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var parameterToken = regexp.MustCompile(`^\$\{params\.([a-z][a-z0-9_-]{0,63})\}$`)

func closedJSON(data []byte, out any) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return err
	}
	if _, err := YAMLJSON(&node); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var tree any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&tree); err != nil {
		return err
	}
	var check func(any, int) error
	check = func(v any, depth int) error {
		if depth > 64 || v == nil {
			return fmt.Errorf("null or excessive JSON nesting is forbidden")
		}
		switch x := v.(type) {
		case map[string]any:
			for _, item := range x {
				if err := check(item, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, item := range x {
				if err := check(item, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return check(tree, 0)
}
func canonicalJSONValue(data []byte) ([]byte, error) {
	var value any
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
func valueHash(value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (p CheckProfile) Hash() string { return valueHash(p) }
func parameterValue(kind string, raw json.RawMessage) (monitor.Scalar, error) {
	var v monitor.Scalar
	if kind == "float" {
		var value any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			return v, fmt.Errorf("finite numeric float parameter required")
		}
		number, ok := value.(json.Number)
		if !ok {
			return v, fmt.Errorf("numeric float parameter required")
		}
		n, err := number.Float64()
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return v, fmt.Errorf("finite float parameter required")
		}
		return monitor.Scalar{Kind: "float", Number: n}, nil
	}
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return v, fmt.Errorf("typed scalar parameter required")
	}
	valid := false
	switch kind {
	case "string":
		valid = v.Kind == "string"
	case "integer":
		valid = v.Kind == "integer"
	case "boolean":
		valid = v.Kind == "boolean"
	case "url":
		u, err := url.Parse(v.Text)
		valid = v.Kind == "string" && err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.User == nil && u.Fragment == ""
	case "ip":
		ip, err := netip.ParseAddr(v.Text)
		valid = v.Kind == "string" && err == nil && ip.Zone() == ""
	case "hostname":
		valid = v.Kind == "string"
		host := strings.TrimSuffix(v.Text, ".")
		if len(host) == 0 || len(host) > 253 || strings.Trim(host, "0123456789.") == "" {
			valid = false
		}
		for _, label := range strings.Split(host, ".") {
			if !hostnameLabel.MatchString(label) {
				valid = false
			}
		}
	}
	if !valid {
		return v, fmt.Errorf("parameter does not match declared type %s", kind)
	}
	if v.Kind == "string" && strings.Contains(v.Text, "${params.") {
		return v, fmt.Errorf("nested parameter substitution is forbidden")
	}
	return v, nil
}

// Validation witnesses check constant constraints with the existing compiler.
// Concrete bindings are always compiled again; witnesses never enter runtime IR.
func parameterWitness(kind, field string) json.RawMessage {
	var value any
	switch kind {
	case "integer":
		value = 443
	case "float":
		return json.RawMessage("1.0")
	case "boolean":
		value = field == "verifyTLS"
	case "url":
		value = "https://192.0.2.1/"
	case "ip":
		value = "192.0.2.1"
	case "hostname":
		value = "probe.example"
	default:
		examples := map[string]string{"url": "https://192.0.2.1/", "method": "GET", "resolver": "192.0.2.53:53", "queryName": "probe.example", "recordType": "A", "transport": "udp", "expectedRcodes": "NOERROR", "preferredIPProtocol": "ip4"}
		value = examples[field]
		if value == "" {
			value = "example"
		}
	}
	b, _ := json.Marshal(value)
	return b
}

// substitute changes values only, never YAML structure, IDs, CEL or map keys.
func substitute(raw json.RawMessage, parameters map[string]ProfileParameter, values map[string]monitor.Scalar, witness bool) (json.RawMessage, error) {
	expandedBytes := len(raw)
	var check map[string]json.RawMessage
	if json.Unmarshal(raw, &check) != nil || check == nil {
		return nil, fmt.Errorf("profile check requires an object")
	}
	var walk func(any, bool, string) (any, error)
	walk = func(value any, allowed bool, field string) (any, error) {
		switch x := value.(type) {
		case string:
			if !strings.Contains(x, "${params.") {
				return x, nil
			}
			match := parameterToken.FindStringSubmatch(x)
			if !allowed || match == nil {
				return nil, fmt.Errorf("only whole-scalar config parameter substitution is allowed")
			}
			definition, found := parameters[match[1]]
			if !found {
				return nil, fmt.Errorf("unknown profile parameter %s", match[1])
			}
			bound, found := values[match[1]]
			if !found && witness {
				var err error
				bound, err = parameterValue(definition.Type, parameterWitness(definition.Type, field))
				if err != nil {
					return nil, err
				}
				found = true
			}
			if !found {
				return nil, fmt.Errorf("missing parameter value %s", match[1])
			}
			data, err := json.Marshal(bound)
			if err != nil {
				return nil, err
			}
			expandedBytes += len(data)
			if expandedBytes > 1<<20 {
				return nil, fmt.Errorf("expanded check exceeds 1 MiB budget")
			}
			return json.RawMessage(data), nil
		case map[string]any:
			for key, item := range x {
				if key == "<<" || strings.Contains(key, "${params.") {
					return nil, fmt.Errorf("template mapping keys and YAML merge are forbidden")
				}
				next, err := walk(item, allowed, key)
				if err != nil {
					return nil, err
				}
				x[key] = next
			}
			return x, nil
		case []any:
			for i, item := range x {
				next, err := walk(item, allowed, field)
				if err != nil {
					return nil, err
				}
				x[i] = next
			}
			return x, nil
		}
		return value, nil
	}
	for key, data := range check {
		var value any
		d := json.NewDecoder(bytes.NewReader(data))
		d.UseNumber()
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		replaced, err := walk(value, key == "config", key)
		if err != nil {
			return nil, err
		}
		check[key], err = json.Marshal(replaced)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(check)
}
func validateProfileCheck(raw json.RawMessage) error {
	stripped, _, err := splitCheckPlacement(raw)
	if err != nil {
		return err
	}
	raw = stripped
	document := map[string]any{"apiVersion": "netmonorc.io/v1alpha1", "kind": "Monitor", "metadata": map[string]string{"name": "profile-validation"}, "spec": map[string]any{"enabled": false, "target": map[string]string{"address": "192.0.2.1"}, "checks": []json.RawMessage{raw}, "assessment": map[string]any{"requiredZones": []string{"validation"}}}}
	data, err := json.Marshal(document)
	if err != nil {
		return err
	}
	_, err = config.Compile(data)
	return err
}
func compileCheckProfile(m Manifest) (CheckProfile, error) {
	var profile CheckProfile
	if err := closedJSON(m.Spec, &profile); err != nil {
		return profile, err
	}
	if len(profile.Checks) == 0 || len(profile.Checks) > 100 || len(profile.Parameters) > 100 || utf8.RuneCountInString(profile.Description) > 1024 {
		return profile, fmt.Errorf("invalid CheckProfile bounds")
	}
	definitions := map[string]ProfileParameter{}
	defaults := map[string]monitor.Scalar{}
	for i, param := range profile.Parameters {
		if !checkIdentifier.MatchString(param.Name) {
			return profile, fmt.Errorf("invalid parameter name")
		}
		if _, exists := definitions[param.Name]; exists {
			return profile, fmt.Errorf("duplicate parameter %s", param.Name)
		}
		switch param.Type {
		case "string", "integer", "float", "boolean", "url", "ip", "hostname":
		default:
			return profile, fmt.Errorf("unsupported parameter type")
		}
		if param.Required && param.Default != nil {
			return profile, fmt.Errorf("required parameter cannot have a default")
		}
		if param.Default != nil {
			v, err := parameterValue(param.Type, param.Default)
			if err != nil {
				return profile, fmt.Errorf("parameter %s: %w", param.Name, err)
			}
			defaults[param.Name] = v
			profile.Parameters[i].Default, _ = json.Marshal(v)
		}
		definitions[param.Name] = param
	}
	ids := map[string]bool{}
	for i, raw := range profile.Checks {
		var header struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &header) != nil || !checkIdentifier.MatchString(header.ID) || ids[header.ID] {
			return profile, fmt.Errorf("invalid or duplicate profile check ID")
		}
		ids[header.ID] = true
		replaced, err := substitute(raw, definitions, defaults, true)
		if err != nil {
			return profile, err
		}
		if err = validateProfileCheck(replaced); err != nil {
			return profile, fmt.Errorf("check %s: %w", header.ID, err)
		}
		profile.Checks[i], err = canonicalJSONValue(raw)
		if err != nil {
			return profile, err
		}
	}
	sort.Slice(profile.Parameters, func(i, j int) bool { return profile.Parameters[i].Name < profile.Parameters[j].Name })
	sort.Slice(profile.Checks, func(i, j int) bool { return string(profile.Checks[i]) < string(profile.Checks[j]) })
	return profile, nil
}
func (p CheckProfile) bind(usage profileUsage) ([]json.RawMessage, monitor.CheckProfileBinding, error) {
	definitions := map[string]ProfileParameter{}
	values := map[string]monitor.Scalar{}
	for _, param := range p.Parameters {
		definitions[param.Name] = param
	}
	for name := range usage.Parameters {
		if _, found := definitions[name]; !found {
			return nil, monitor.CheckProfileBinding{}, fmt.Errorf("unknown parameter %s", name)
		}
	}
	for _, param := range p.Parameters {
		raw, provided := usage.Parameters[param.Name]
		if !provided {
			if param.Required {
				return nil, monitor.CheckProfileBinding{}, fmt.Errorf("required parameter %s is missing", param.Name)
			}
			raw = param.Default
		}
		if raw == nil {
			continue
		}
		v, err := parameterValue(param.Type, raw)
		if err != nil {
			return nil, monitor.CheckProfileBinding{}, fmt.Errorf("parameter %s: %w", param.Name, err)
		}
		values[param.Name] = v
	}
	result := []json.RawMessage{}
	expandedBytes := 0
	for _, raw := range p.Checks {
		expanded, err := substitute(raw, definitions, values, false)
		if err != nil {
			return nil, monitor.CheckProfileBinding{}, err
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(expanded, &fields)
		var id string
		_ = json.Unmarshal(fields["id"], &id)
		id = usage.As + "__" + id
		if !checkIdentifier.MatchString(id) {
			return nil, monitor.CheckProfileBinding{}, fmt.Errorf("useProfiles produces an ID longer than 64 characters")
		}
		fields["id"], _ = json.Marshal(id)
		expanded, _ = json.Marshal(fields)
		expandedBytes += len(expanded)
		if expandedBytes > 1<<20 {
			return nil, monitor.CheckProfileBinding{}, fmt.Errorf("expanded profile exceeds 1 MiB budget")
		}
		result = append(result, expanded)
	}
	return result, monitor.CheckProfileBinding{Ref: usage.Ref, As: usage.As, Hash: p.Hash(), ParametersHash: valueHash(values)}, nil
}
func (r Registry) expandProfiles(m Manifest) (Manifest, []monitor.CheckProfileBinding, error) {
	canonical, err := canonicalJSONValue(m.Spec)
	if err != nil {
		return m, nil, err
	}
	if bytes.Contains(canonical, []byte("${params.")) {
		return m, nil, fmt.Errorf("parameter template syntax outside CheckProfile is forbidden")
	}
	var spec map[string]json.RawMessage
	if json.Unmarshal(m.Spec, &spec) != nil || spec == nil {
		return m, nil, fmt.Errorf("Monitor.spec requires an object")
	}
	raw, present := spec["useProfiles"]
	if !present {
		return m, nil, nil
	}
	var usages []profileUsage
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || closedJSON(raw, &usages) != nil || len(usages) > 100 {
		return m, nil, fmt.Errorf("invalid useProfiles list")
	}
	var checks []json.RawMessage
	if inline, found := spec["checks"]; found {
		if bytes.Equal(bytes.TrimSpace(inline), []byte("null")) || json.Unmarshal(inline, &checks) != nil {
			return m, nil, fmt.Errorf("checks requires a list")
		}
	}
	if len(checks) > 100 {
		return m, nil, fmt.Errorf("at most 100 expanded checks supported")
	}
	expandedBytes := 0
	for _, check := range checks {
		expandedBytes += len(check)
	}
	bindings := []monitor.CheckProfileBinding{}
	for i, usage := range usages {
		if !namePattern.MatchString(usage.Ref) || !checkIdentifier.MatchString(usage.As) {
			return m, nil, fmt.Errorf("useProfiles[%d] requires valid ref/as", i)
		}
		profile, found := r.CheckProfiles[usage.Ref]
		if !found {
			return m, nil, fmt.Errorf("useProfiles[%d]: CheckProfile unavailable", i)
		}
		if len(checks)+len(profile.Checks) > 100 {
			return m, nil, fmt.Errorf("useProfiles[%d] exceeds 100 expanded checks", i)
		}
		expanded, binding, err := profile.bind(usage)
		if err != nil {
			return m, nil, fmt.Errorf("useProfiles[%d]: %w", i, err)
		}
		for _, check := range expanded {
			expandedBytes += len(check)
		}
		if expandedBytes > 1<<20 {
			return m, nil, fmt.Errorf("expanded Monitor checks exceed 1 MiB budget")
		}
		checks = append(checks, expanded...)
		bindings = append(bindings, binding)
	}
	delete(spec, "useProfiles")
	spec["checks"], _ = json.Marshal(checks)
	m.Spec, _ = json.Marshal(spec)
	return m, bindings, nil
}
