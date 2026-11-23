package diagnostics

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"time"
)

type DiagnosticCounters struct {
	Attempts        int64   `cel:"attempts"`
	RemainingChecks int64   `cel:"remainingChecks"`
	ElapsedSeconds  float64 `cel:"elapsedSeconds"`
}
type FixedContext struct {
	Monitor    monitor.RuntimeMonitorView
	Target     monitor.TargetView
	Zone       *monitor.EvaluatedZoneView
	Zones      map[string]monitor.EvaluatedZoneView
	Checks     map[string]monitor.CheckView
	Diagnostic DiagnosticCounters
}
type SessionContext struct {
	Plan         monitor.Plan
	Assessment   monitor.Assessment
	Observations []monitor.Observation
	Slots        []monitor.Slot
	Executors    []Executor
}

func (c *SessionContext) contextFor(action Action, state string) FixedContext {
	frame := FixedContext{Monitor: monitor.RuntimeMonitorView{State: state, CoverageState: "unknown", Labels: map[string]string{}}, Target: monitor.TargetCELContext(monitor.Plan{}), Zones: map[string]monitor.EvaluatedZoneView{}}
	if c == nil {
		return frame
	}
	a, err := monitor.Evaluate(c.Plan, c.Slots, c.Observations, time.Now())
	if err != nil {
		a = monitor.Assessment{Monitor: c.Plan.Name, State: "unknown"}
	}
	frame.Monitor = monitor.MonitorContext(c.Plan, a)
	frame.Target = monitor.TargetCELContext(c.Plan)
	for _, zone := range a.Zones {
		coverage := "degraded"
		if zone.Coverage == "complete" {
			coverage = "sufficient"
		}
		if len(zone.Evidence) == 0 {
			coverage = "unknown"
		}
		frame.Zones[zone.Zone] = monitor.EvaluatedZoneView{Name: zone.Zone, State: zone.State, Coverage: coverage, Labels: map[string]string{}, Checks: zone.Checks, WeightedScore: zone.H}
	}
	names := map[string]bool{}
	for _, zone := range action.Placement.Zones {
		names[zone] = true
	}
	if len(names) == 0 {
		for _, agent := range c.Executors {
			if placementMatch(action.Placement, agent) {
				names[agent.Zone] = true
			}
		}
	}
	if len(names) == 0 && len(a.Zones) == 1 {
		names[a.Zones[0].Zone] = true
	}
	// Bare checks/zone are deliberately absent for an ambiguous perspective.
	if len(names) == 1 {
		for name := range names {
			if zone, ok := frame.Zones[name]; ok {
				frame.Zone = &zone
				frame.Checks = zone.Checks
			}
		}
	}
	return frame
}
