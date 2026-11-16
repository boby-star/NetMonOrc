package placement

import (
	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
	"time"
)

func fixtures() (monitor.Plan, []agentstatus.View, time.Time) {
	now := time.Now()
	expiry := now.Add(time.Hour)
	capacity, active := 2, 0
	slots := []monitor.Slot{{Agent: "branch", Zone: "office"}, {Agent: "dc", Zone: "server"}}
	plan := monitor.Plan{Checks: []monitor.Check{{ID: "http", Type: "http", Runner: "native-http", URL: "http://198.18.0.80/"}}, FixedPlacement: &monitor.FixedPlacement{Strategy: "leastLoaded", Replicas: 1, Candidates: slots}}
	var agents []agentstatus.View
	for _, slot := range slots {
		agents = append(agents, agentstatus.View{AgentID: slot.Agent, NetworkZone: slot.Zone, OnlineState: "online", LastHeartbeat: &now, CertExpiresAt: &expiry, MaxConcurrentProbes: &capacity, ActiveProbes: &active, Capabilities: []string{"http", "native-http", "placement:dynamic", "target-cidr:198.18.0.0/24"}})
	}
	return plan, agents, now
}
func TestLoadRatioAndStickyAssignment(t *testing.T) {
	p, agents, now := fixtures()
	busy := 1
	agents[0].ActiveProbes = &busy
	selected, next := Select(p, agents, 0, now)
	if len(selected) != 1 || selected[0].Agent != "dc" || next != 0 {
		t.Fatal(selected, next)
	}
	p.FixedPlacement.Selected = selected
	agents[0].ActiveProbes = agents[1].ActiveProbes
	agents[1].ActiveProbes = &busy
	selected, _ = Select(p, agents, 0, now)
	if selected[0].Agent != "dc" {
		t.Fatal("heartbeat caused unnecessary reassignment")
	}
	agents[1].OnlineState = "offline"
	selected, _ = Select(p, agents, 0, now)
	if len(selected) != 1 || selected[0].Agent != "branch" {
		t.Fatal("lost Agent not replaced", selected)
	}
}
func TestRoundRobinCursorAndPendingHardConstraints(t *testing.T) {
	p, agents, now := fixtures()
	p.FixedPlacement.Strategy = "roundRobin"
	first, cursor := Select(p, agents, 0, now)
	second, next := Select(p, agents, cursor, now)
	if first[0].Agent != "branch" || second[0].Agent != "dc" || next != 0 {
		t.Fatal(first, second, cursor, next)
	}
	p.FixedPlacement.Replicas = 2
	agents[1].Capabilities = []string{"http", "native-http", "placement:dynamic", "target-cidr:10.0.0.0/8"}
	selected, unchanged := Select(p, agents, cursor, now)
	if len(selected) != 0 || unchanged != cursor {
		t.Fatal("partial or policy-denied set scheduled", selected)
	}
	p.FixedPlacement.Replicas = 1
	agents[0].MaxConcurrentProbes = nil
	selected, _ = Select(p, agents, 0, now)
	if len(selected) != 0 {
		t.Fatal("capacity guessed or forbidden target admitted")
	}
}

func TestDistinctZonesFilterPhysicalCandidates(t *testing.T) {
	p, agents, now := fixtures()
	p.FixedPlacement.Replicas = 2
	p.FixedPlacement.DistinctBy = "zone"
	p.FixedPlacement.Candidates[1].Zone = "office"
	agents[1].NetworkZone = "office"
	for _, mode := range []string{"leastLoaded", "roundRobin"} {
		p.FixedPlacement.Strategy = mode
		selected, next := Select(p, agents, 0, now)
		if len(selected) != 0 || next != 0 {
			t.Fatal("partial distinct-zone set scheduled", mode, selected)
		}
	}
	third := agents[1]
	third.AgentID = "edge"
	third.NetworkZone = "server"
	agents = append(agents, third)
	p.FixedPlacement.Candidates = append(p.FixedPlacement.Candidates, monitor.Slot{Agent: "edge", Zone: "server"})
	for _, mode := range []string{"leastLoaded", "roundRobin"} {
		p.FixedPlacement.Strategy = mode
		selected, _ := Select(p, agents, 0, now)
		if len(selected) != 2 || selected[0].Zone == selected[1].Zone {
			t.Fatal("feasible distinct-zone set not selected", mode, selected)
		}
	}
}
