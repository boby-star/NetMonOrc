package agent

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/component"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/probe"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
)

func monitorComponents(ctx context.Context, st *state, id string) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		refreshComponents(ctx, st, id, probe.ComponentReady)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func refreshComponents(ctx context.Context, st *state, id string, check func(context.Context, monitor.ComponentBinding) bool) {
	assignments := st.list()
	bindings := map[string]monitor.ComponentBinding{}
	for _, a := range assignments {
		if b, ok := a.plan.Components[id]; ok && b.Enabled {
			bindings[b.Endpoint] = b
		}
	}
	// Group repeated Plans by endpoint; at most four bounded health requests,
	// independently of probe slots, and never blocking stream/lease renewal.
	child, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	var mu sync.Mutex
	results := map[string]component.Readiness{}
	var workers sync.WaitGroup
	semaphore := make(chan struct{}, 4)
	endpoints := make([]string, 0, len(bindings))
	for endpoint := range bindings {
		endpoints = append(endpoints, endpoint)
	}
	sort.Strings(endpoints)
	for _, endpoint := range endpoints {
		b := bindings[endpoint]
		workers.Add(1)
		go func() {
			defer workers.Done()
			ready := false
			select {
			case semaphore <- struct{}{}:
				ready = check(child, b)
				<-semaphore
			case <-child.Done():
			}
			r := component.Readiness{State: "unavailable", Reason: "endpoint_unavailable", CheckedAt: time.Now().UTC()}
			if ready {
				r.State = "ready"
				r.Reason = "endpoint_ready"
			}
			mu.Lock()
			results[b.Endpoint] = r
			mu.Unlock()
		}()
	}
	workers.Wait()
	samples := map[string]component.Readiness{}
	for _, a := range assignments {
		b, ok := a.plan.Components[id]
		if !ok {
			continue
		}
		r := results[b.Endpoint]
		if !b.Enabled {
			r = component.Readiness{State: "disabled", Reason: "binding_disabled", CheckedAt: time.Now().UTC()}
		}
		r.Revision = a.plan.Revision
		r.Ref = b.Ref
		r.Hash = b.Hash
		samples[r.Revision] = r
	}
	st.mu.Lock()
	st.components = samples
	st.mu.Unlock()
}

func componentReports(st *state, id string, applied map[string]bool) []*pb.ComponentReadiness {
	assignments := st.list()
	st.mu.Lock()
	defer st.mu.Unlock()
	var reports []*pb.ComponentReadiness
	for _, a := range assignments {
		b, explicit := a.plan.Components[id]
		r, ok := st.components[a.plan.Revision]
		if !applied[a.plan.Revision] || !explicit || !ok || r.Ref != b.Ref || r.Hash != b.Hash || !component.Fresh(r.CheckedAt, time.Now()) {
			continue
		}
		reports = append(reports, &pb.ComponentReadiness{Revision: r.Revision, ComponentRef: r.Ref, BindingHash: r.Hash, State: r.State, Reason: r.Reason, CheckedAtUnixMilli: r.CheckedAt.UnixMilli()})
	}
	return reports
}
