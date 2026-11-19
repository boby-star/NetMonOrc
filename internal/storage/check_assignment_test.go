package storage

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPerCheckHistoricalAssignmentReplay(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires real PostgreSQL DSN file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	s, err := Open(ctx, strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	agent := "check-replay-" + newID()
	name := agent
	p := monitor.Plan{Name: name, Revision: "old-scoped", Checks: []monitor.Check{{ID: "owned"}, {ID: "other"}}, CheckPlacements: map[string]*monitor.FixedPlacement{"owned": {Selected: []monitor.Slot{{Agent: agent, Zone: "office"}}}, "other": {Selected: []monitor.Slot{{Agent: "another", Zone: "office"}}}}}
	generation, err := s.BeginSession(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AssignPlan(ctx, agent, generation, p, "office"); err != nil {
		t.Fatal(err)
	}
	// Reconnect into a new revision; old regular results still use old ownership.
	generation, err = s.BeginSession(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Assign(ctx, agent, generation, "new", "office", name); err != nil {
		t.Fatal(err)
	}
	if err = s.Applied(ctx, agent, generation, "new"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	o := monitor.Observation{ID: newID(), Monitor: name, Revision: p.Revision, Check: "other", Agent: agent, Zone: "office", StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "success"}
	if _, err = s.CommitBatch(ctx, agent, generation, "new", []monitor.Observation{o}); err == nil {
		t.Fatal("old revision replay bypassed per-check ownership")
	}
	o.ID = newID()
	o.Check = "owned"
	if _, err = s.CommitBatch(ctx, agent, generation, "new", []monitor.Observation{o}); err != nil {
		t.Fatal("legitimate old-revision replay rejected", err)
	}
}
