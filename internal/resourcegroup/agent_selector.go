package resourcegroup

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var agentLabelKey = regexp.MustCompile(`^[a-z0-9.-]+$`)

func ValidAgentLabel(key, value string) bool {
	return agentLabelKey.MatchString(key) && len(key) <= 63 && utf8.RuneCountInString(value) <= 63 && strings.ToLower(value) == value
}
func NormalizeAgentSelector(source *Selector) (*Selector, error) {
	if source == nil {
		return nil, nil
	}
	if len(source.MatchLabels) > 64 || len(source.MatchExpressions) > 64 {
		return nil, fmt.Errorf("Agent selector exceeds limits")
	}
	if len(source.MatchLabels) == 0 && len(source.MatchExpressions) == 0 {
		return nil, nil
	}
	result := &Selector{MatchLabels: map[string]string{}, MatchExpressions: append([]Expression(nil), source.MatchExpressions...)}
	for key, value := range source.MatchLabels {
		if !ValidAgentLabel(key, value) {
			return nil, fmt.Errorf("invalid selector matchLabels")
		}
		result.MatchLabels[key] = value
	}
	for i := range result.MatchExpressions {
		e := &result.MatchExpressions[i]
		if !ValidAgentLabel(e.Key, "") || len(e.Values) > 100 {
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
			if !ValidAgentLabel(e.Key, v) || seen[v] {
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
