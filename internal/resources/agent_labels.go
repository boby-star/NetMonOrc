package resources

import (
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
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
	return resourcegroup.NormalizeAgentSelector(source)
}
