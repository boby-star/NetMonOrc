package main

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/boby-star/NetMonOrc/internal/coordinator"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/notify"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/boby-star/NetMonOrc/internal/storage"
	"github.com/boby-star/NetMonOrc/internal/transport"
)

type monitorRuntime struct {
	plan          monitor.Plan
	assessor      *monitor.Assessor
	slots         []monitor.Slot
	assigned      []monitor.Slot
	diagnostic    *coordinator.Coordinator
	notifications *notify.Service
}

// The bootstrap Monitor retains its existing snapshot filenames. Additional
// Monitors have distinct files; their authoritative live state remains in SQL.
func runtimePath(path, name, primary string) string {
	if path == "" || name == primary {
		return path
	}
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "." + name + ext
}
func runtimePlans(compiled map[string]resources.Compiled) []monitor.Plan {
	var plans []monitor.Plan
	for _, v := range compiled {
		if v.Monitor != nil {
			plans = append(plans, *v.Monitor)
		}
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Name < plans[j].Name })
	return plans
}
func addRuntimes(ctx context.Context, primary monitorRuntime, plans []monitor.Plan, registry resources.Registry, server *transport.Server, store *storage.Store, reportPath string) (map[string]*monitorRuntime, error) {
	out := map[string]*monitorRuntime{primary.plan.Name: &primary}
	for _, plan := range plans {
		if plan.Name == primary.plan.Name {
			continue
		}
		assessor, err := monitor.NewAssessor(plan)
		if err != nil {
			return out, err
		}
		runtime := &monitorRuntime{plan: plan, assessor: assessor, slots: plan.AssessmentSlots(server.Slots), assigned: plan.RegularSlots(server.Slots)}
		out[plan.Name] = runtime
		if primary.notifications != nil {
			n := primary.notifications
			runtime.notifications = &notify.Service{Store: store, Managed: true, Channels: n.Channels, Endpoint: n.Endpoint, Sender: n.Sender}
		}
		if plan.Diagnostics != nil && plan.Diagnostics.Enabled {
			runtime.diagnostic, err = coordinator.New(plan, registry.Profiles[plan.Diagnostics.ProfileRef], server, store, runtimePath(reportPath, plan.Name, primary.plan.Name))
			if err != nil {
				return out, err
			}
			if err = runtime.diagnostic.RestoreReport(ctx); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}
