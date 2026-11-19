package resources

import (
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"sort"
	"strings"
	"unicode/utf8"
)

func validAgentLabel(key, value string) bool {
	return labelKey.MatchString(key) && len(key) <= 63 && utf8.RuneCountInString(value) <= 63 && strings.ToLower(value) == value
}

// ParseAgentLabels reads a Controller-owned inventory, never Agent Hello data.
func ParseAgentLabels(data []byte, roster []monitor.Slot) (map[string]map[string]string, error) {
	var out map[string]map[string]string
	if err := DecodeJSON(data, &out); err != nil {
		return nil, fmt.Errorf("invalid agent labels inventory: %w", err)
	}
	if out == nil || len(out) > 100 {
		return nil, fmt.Errorf("agent labels require a bounded object")
	}
	known := map[string]bool{}
	for _, slot := range roster {
		known[slot.Agent] = true
	}
	for id, labels := range out {
		if !known[id] || labels == nil || len(labels) > 64 {
			return nil, fmt.Errorf("agent labels require known roster IDs and at most 64 labels")
		}
		for key, value := range labels {
			if !validAgentLabel(key, value) {
				return nil, fmt.Errorf("invalid Agent label")
			}
		}
	}
	return out, nil
}

func normalizeAgentSelector(source *resourcegroup.Selector) (*resourcegroup.Selector, error) {
	if source == nil {
		return nil, nil
	}
	if len(source.MatchLabels) > 64 || len(source.MatchExpressions) > 64 {
		return nil, fmt.Errorf("Agent selector exceeds limits")
	}
	if len(source.MatchLabels) == 0 && len(source.MatchExpressions) == 0 {
		return nil, nil
	}
	result := &resourcegroup.Selector{MatchLabels: map[string]string{}, MatchExpressions: append([]resourcegroup.Expression(nil), source.MatchExpressions...)}
	for key, value := range source.MatchLabels {
		if !validAgentLabel(key, value) {
			return nil, fmt.Errorf("invalid selector matchLabels")
		}
		result.MatchLabels[key] = value
	}
	for i := range result.MatchExpressions {
		e := &result.MatchExpressions[i]
		if !validAgentLabel(e.Key, "") || len(e.Values) > 100 {
			return nil, fmt.Errorf("invalid selector expression")
		}
		switch e.Operator {
		case "In", "NotIn":
			if len(e.Values) == 0 {
				return nil, fmt.Errorf("selector values required")
			}
		case "Exists", "DoesNotExist":
			if len(e.Values) != 0 {
				return nil, fmt.Errorf("selector values forbidden")
			}
		default:
			return nil, fmt.Errorf("invalid selector operator")
		}
		e.Values = append([]string(nil), e.Values...)
		seen := map[string]bool{}
		for _, v := range e.Values {
			if !validAgentLabel(e.Key, v) || seen[v] {
				return nil, fmt.Errorf("invalid or duplicate selector value")
			}
			seen[v] = true
		}
		sort.Strings(e.Values)
	}
	sort.Slice(result.MatchExpressions, func(i, j int) bool {
		a, _ := json.Marshal(result.MatchExpressions[i])
		b, _ := json.Marshal(result.MatchExpressions[j])
		return string(a) < string(b)
	})
	return result, nil
}
