package resources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/component"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
)

type Component struct {
	Driver        string                  `json:"driver"`
	Mode          string                  `json:"mode"`
	AgentSelector *resourcegroup.Selector `json:"agentSelector"`
	Endpoint      string                  `json:"endpoint"`
	ModuleSet     string                  `json:"moduleSet"`
	Enabled       bool                    `json:"enabled"`
	Description   string                  `json:"description,omitempty"`
}

func (c Component) Hash() string { c.Description = ""; return valueHash(c) }

func compileComponent(m Manifest) (Component, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(m.Spec, &fields); err != nil {
		return Component{}, err
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return Component{}, fmt.Errorf("Component fields must not be null")
		}
	}
	var wire struct {
		Driver        string                  `json:"driver"`
		Mode          string                  `json:"mode"`
		AgentSelector *resourcegroup.Selector `json:"agentSelector"`
		Endpoint      string                  `json:"endpoint"`
		ModuleSet     string                  `json:"moduleSet"`
		Enabled       *bool                   `json:"enabled"`
		Description   string                  `json:"description"`
	}
	if err := closedJSON(m.Spec, &wire); err != nil {
		return Component{}, err
	}
	c := Component{Driver: wire.Driver, Mode: wire.Mode, AgentSelector: wire.AgentSelector, Endpoint: wire.Endpoint, ModuleSet: wire.ModuleSet, Enabled: true, Description: wire.Description}
	if wire.Enabled != nil {
		c.Enabled = *wire.Enabled
	}
	if c.Mode == "" {
		c.Mode = "external"
	}
	if c.ModuleSet == "" {
		c.ModuleSet = component.ModuleSet
	}
	if c.Driver != "blackbox" || c.Mode != "external" || c.ModuleSet != component.ModuleSet || c.AgentSelector == nil || utf8.RuneCountInString(c.Description) > 1024 {
		return c, fmt.Errorf("Component requires blackbox/external, standard-network and agentSelector")
	}
	selector, err := normalizeAgentSelector(c.AgentSelector)
	if err != nil {
		return c, err
	}
	if selector == nil {
		selector = &resourcegroup.Selector{}
	}
	c.AgentSelector = selector
	c.Endpoint, err = component.NormalizeEndpoint(c.Endpoint)
	return c, err
}

func (r Registry) bindComponents(p monitor.Plan) (monitor.Plan, error) {
	names := make([]string, 0, len(r.Components))
	for name := range r.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	bindings := map[string]monitor.ComponentBinding{}
	for _, slot := range r.Roster {
		for _, name := range names {
			c := r.Components[name]
			if !c.AgentSelector.Match(r.AgentLabels[slot.Agent]) {
				continue
			}
			if _, exists := bindings[slot.Agent]; exists {
				return p, fmt.Errorf("overlapping Component selectors for Agent %s", slot.Agent)
			}
			bindings[slot.Agent] = monitor.ComponentBinding{Ref: name, Hash: c.Hash(), Endpoint: c.Endpoint, ModuleSet: c.ModuleSet, Enabled: c.Enabled}
		}
	}
	if len(bindings) == 0 {
		return p, nil
	}
	return config.ResolveComponents(p, bindings)
}
