package storage

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestFallbackDurableOwnershipAndNoBudgetRefund(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires real PostgreSQL DSN file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, err := Open(ctx, strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	name := "fallback-" + newID()
	root := monitor.Slot{Agent: name + "-r", Zone: "office"}
	spare := monitor.Slot{Agent: name + "-s", Zone: "office"}
	check := monitor.Check{ID: "http", Type: "http", Runner: "native-http", Method: "GET", URL: "http://127.0.0.1/", ExpectedStatus: []int{200}, MaxBodyBytes: 16384, Timeout: time.Second, Interval: 10 * time.Second, Fallback: &monitor.FallbackPolicy{On: []string{"agent_unreachable"}, Strategy: "same-zone", MaxAttempts: 1}}
	p := monitor.Plan{Name: name, Revision: "fallback-revision", MaxAge: time.Minute, Checks: []monitor.Check{check}, FallbackRoster: []monitor.Slot{root, spare}, FixedPlacement: &monitor.FixedPlacement{Selected: []monitor.Slot{root}}}
	gen, err := s.BeginSession(ctx, spare.Agent)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AssignPlan(ctx, spare.Agent, gen, p, spare.Zone); err != nil {
		t.Fatal(err)
	}
	if err = s.Applied(ctx, spare.Agent, gen, p.Revision); err != nil {
		t.Fatal(err)
	}
	if recorded, err := s.RecordConfigRejection(ctx, spare.Agent, gen-1, p.Revision); err != nil || recorded {
		t.Fatal("stale generation recorded rejection", recorded, err)
	}
	if recorded, err := s.RecordConfigRejection(ctx, spare.Agent, gen, "not-assigned"); err != nil || recorded {
		t.Fatal("unknown assignment recorded rejection", recorded, err)
	}
	if recorded, err := s.RecordConfigRejection(ctx, spare.Agent, gen, p.Revision); err != nil || !recorded {
		t.Fatal("current assignment rejection not recorded", recorded, err)
	}
	if err = s.Applied(ctx, spare.Agent, gen, p.Revision); err != nil {
		t.Fatal(err)
	}
	if rejected, err := s.ConfigRejected(ctx, spare.Agent, p.Revision); err != nil || rejected {
		t.Fatal("successful apply retained stale rejection", rejected, err)
	}
	r, err := s.OpenFallback(ctx, name, FallbackRun{Monitor: name, Revision: p.Revision, Check: check.ID, OriginAgent: root.Agent, OriginZone: root.Zone, Cause: "agent_unreachable", Strategy: "same-zone", MaxAttempts: 1, StartedAt: time.Now(), Deadline: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	task := DiagnosticTask{ID: newID(), RunID: r.ID, Agent: spare.Agent, Zone: spare.Zone, Generation: gen, Plan: p, Check: config.FallbackCheck(check), AttemptNo: 1, Deadline: time.Now().Add(10 * time.Second)}
	if err = s.ReserveFallback(ctx, task); err != nil {
		t.Fatal(err)
	}
	second := task
	second.ID = newID()
	second.AttemptNo = 2
	if err = s.ReserveFallback(ctx, second); err == nil {
		t.Fatal("fallback maxAttempts bypassed")
	}
	now := time.Now()
	o := monitor.Observation{ID: task.ID, FallbackTaskID: task.ID, Agent: spare.Agent, Zone: spare.Zone, Monitor: name, Revision: p.Revision, Check: check.ID, StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "success"}
	forged := o
	forged.Check = "forged"
	if _, err = s.CommitBatch(ctx, spare.Agent, gen, p.Revision, []monitor.Observation{forged}); err == nil {
		t.Fatal("unbound fallback result accepted")
	}
	if _, err = s.CommitBatch(ctx, spare.Agent, gen, p.Revision, []monitor.Observation{o}); err != nil {
		t.Fatal("idle spare's reserved result rejected", err)
	}
	duplicate, err := s.CommitBatch(ctx, spare.Agent, gen, p.Revision, []monitor.Observation{o})
	if err != nil || !duplicate[0] {
		t.Fatal("fallback replay not idempotent", err)
	}
	if regular, err := s.LatestRegular(ctx, p, time.Now()); err != nil || len(regular) != 0 {
		t.Fatal("fallback became ordinary regular observation", err)
	}
	if recovered, err := s.Latest(ctx, p, time.Now()); err != nil || len(recovered) != 1 || recovered[0].Agent != spare.Agent || recovered[0].FallbackForAgent != root.Agent {
		t.Fatal("coverage provenance lost", recovered, err)
	}
	view, found, err := s.ReadFallback(ctx, r.ID)
	if err != nil || !found || view.Used != 1 || len(view.Tasks) != 1 || view.Tasks[0].ObservationID != o.ID {
		t.Fatal("durable fallback journal missing", view, err)
	}
	reopened, err := s.OpenFallback(ctx, name, r)
	if err != nil || reopened.ID != r.ID || reopened.Used != 1 {
		t.Fatal("root replay replenished budget", reopened, err)
	}
	expired := r
	expired.Deadline = time.Now().Add(-time.Second)
	expired.Used = 0
	expired, err = s.OpenFallback(ctx, name+"-expired", expired)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ExpireFallbacks(ctx); err != nil {
		t.Fatal(err)
	}
	expired, err = s.OpenFallback(ctx, name+"-expired", expired)
	if err != nil || expired.State != "expired" || expired.Used != 0 {
		t.Fatal("expired root reopened or spent a task", expired, err)
	}
}
