package resources

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/boby-star/NetMonOrc/internal/config"
)

type AssessmentPolicy struct{ Spec json.RawMessage }

func (a AssessmentPolicy) Hash() string { return valueHash(a.Spec) }
func compileAssessmentPolicy(m Manifest) (AssessmentPolicy, error) {
	var node map[string]json.RawMessage
	if json.Unmarshal(m.Spec, &node) != nil || node == nil {
		return AssessmentPolicy{}, fmt.Errorf("AssessmentPolicy.spec requires an object")
	}
	// Validate duplicate keys, nulls and nesting before the typed YAML decoder.
	var raw any
	if err := closedJSON(m.Spec, &raw); err != nil {
		return AssessmentPolicy{}, err
	}
	data, err := executionJSON(m)
	if err != nil {
		return AssessmentPolicy{}, err
	}
	normalized, err := config.NormalizeAssessmentPolicy(data)
	if err != nil {
		return AssessmentPolicy{}, err
	}
	if bytes.Contains(normalized, []byte("${params.")) {
		return AssessmentPolicy{}, fmt.Errorf("parameter templates are forbidden in AssessmentPolicy")
	}
	return AssessmentPolicy{Spec: normalized}, nil
}
func (r Registry) resolveAssessment(m Manifest) (Manifest, string, string, error) {
	var spec map[string]json.RawMessage
	if json.Unmarshal(m.Spec, &spec) != nil || spec == nil {
		return m, "", "", fmt.Errorf("Monitor.spec requires an object")
	}
	refRaw, linked := spec["assessmentRef"]
	inline, hasInline := spec["assessment"]
	var ref, hash string
	if linked {
		if hasInline {
			return m, "", "", fmt.Errorf("Monitor.spec cannot contain both assessment and assessmentRef")
		}
		if json.Unmarshal(refRaw, &ref) != nil || !namePattern.MatchString(ref) {
			return m, "", "", fmt.Errorf("assessmentRef requires a resource name")
		}
		policy, found := r.Assessments[ref]
		if !found {
			return m, "", "", fmt.Errorf("AssessmentPolicy %s is unavailable", ref)
		}
		inline = policy.Spec
		hash = policy.Hash()
		delete(spec, "assessmentRef")
	} else if !hasInline {
		inline = json.RawMessage(`{}`)
	}
	var assessment map[string]json.RawMessage
	if json.Unmarshal(inline, &assessment) != nil || assessment == nil {
		return m, "", "", fmt.Errorf("assessment requires an object")
	}
	if _, explicit := assessment["requiredZones"]; !explicit {
		if len(r.Zones) == 0 {
			return m, "", "", fmt.Errorf("assessment.requiredZones cannot be inferred without a configured roster")
		}
		assessment["requiredZones"], _ = json.Marshal(r.Zones)
	}
	spec["assessment"], _ = json.Marshal(assessment)
	m.Spec, _ = json.Marshal(spec)
	return m, ref, hash, nil
}
