package main

import (
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"net/netip"
	"os"
	"sort"
	"strings"
)

// The local path uses the same catalog resolver as Controller validate/apply.
// It checks supplied manifests only, never claiming access to the live roster.
func localValidate(data []byte, slotsArg, cidrsArg string, labelsFile ...string) error {
	batch, err := parseBatch(data)
	if err != nil {
		return err
	}
	registry := resources.Registry{ManagedWebhooks: true}
	zones := map[string]bool{}
	agents := map[string]bool{}
	if slotsArg != "" {
		for _, pair := range strings.Split(slotsArg, ",") {
			id, zone, ok := strings.Cut(pair, "=")
			if !ok || id == "" || zone == "" || agents[id] {
				return fmt.Errorf("--slots requires Agent=zone pairs")
			}
			agents[id] = true
			registry.Roster = append(registry.Roster, monitor.Slot{Agent: id, Zone: zone})
			zones[zone] = true
		}
	}
	if len(labelsFile) > 0 && labelsFile[0] != "" {
		inventory, e := os.ReadFile(labelsFile[0])
		if e != nil {
			return e
		}
		registry.AgentLabels, e = resources.ParseAgentLabels(inventory, registry.Roster)
		if e != nil {
			return e
		}
	}
	// A zone placeholder permits schema/type validation of defaulted policies;
	// it is disclosed in output and never becomes a Controller assignment.
	abstract := len(zones) == 0
	if abstract {
		zones["validation-only"] = true
	}
	for zone := range zones {
		registry.Zones = append(registry.Zones, zone)
	}
	sort.Strings(registry.Zones)
	if cidrsArg != "" {
		for _, value := range strings.Split(cidrsArg, ",") {
			prefix, e := netip.ParsePrefix(value)
			if e != nil {
				return fmt.Errorf("invalid --allow-cidrs")
			}
			registry.WebhookCIDRs = append(registry.WebhookCIDRs, prefix)
		}
	}
	_, compiled, err := resources.ResolveCatalog(registry, nil, batch.Manifests)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(compiled))
	for key := range compiled {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	summary := []map[string]string{}
	for _, key := range keys {
		summary = append(summary, map[string]string{"resource": key, "validationRevision": compiled[key].Revision})
	}
	result := map[string]any{"valid": true, "mode": "offline", "planningRequired": true, "abstractZoneContext": abstract, "resources": summary, "warnings": []string{"Only the supplied catalog was validated. Remote plan/apply resolves the Controller catalog, permissions and current assignments."}}
	return json.NewEncoder(os.Stdout).Encode(result)
}
