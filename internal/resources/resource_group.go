package resources

import (
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"sort"
)

func compileGroup(raw json.RawMessage) (resourcegroup.Spec, error) {
	var s resourcegroup.Spec
	if err := closedJSON(raw, &s); err != nil {
		return s, err
	}
	if s.Selector != nil {
		for k := range s.Selector.MatchLabels {
			if !labelKey.MatchString(k) {
				return s, fmt.Errorf("invalid selector label key")
			}
		}
		for _, e := range s.Selector.MatchExpressions {
			if !labelKey.MatchString(e.Key) {
				return s, fmt.Errorf("invalid selector expression key")
			}
		}
	}
	if err := s.Validate(); err != nil {
		return s, err
	}
	return s, nil
}
func bindGroup(s resourcegroup.Spec, all map[string]Manifest, compiled map[string]Compiled) ([]string, string, error) {
	ids := []string{}
	if s.Members != nil {
		ids = append(ids, (*s.Members)...)
	} else {
		for _, m := range all {
			if m.Kind == "Monitor" && s.Selector.Match(m.Metadata.Labels) {
				ids = append(ids, m.Metadata.Name)
			}
		}
	}
	sort.Strings(ids)
	if len(ids) > 100 {
		return nil, "", fmt.Errorf("group exceeds current capacity of 100 members")
	}
	bindings := []any{}
	for _, id := range ids {
		c, ok := compiled["Monitor/"+id]
		if !ok || c.Monitor == nil {
			return nil, "", fmt.Errorf("group Monitor %s is unavailable", id)
		}
		bindings = append(bindings, struct{ Name, Revision string }{id, c.Revision})
	}
	return ids, valueHash(struct {
		Spec     resourcegroup.Spec
		Bindings []any
	}{s, bindings}), nil
}
