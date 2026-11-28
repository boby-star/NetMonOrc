package storage

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestPostgresResponseAdmissionAndFencing(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires real PostgreSQL")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store, err := Open(ctx, strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	name := "response-" + time.Now().Format("150405000000")
	plan := monitor.Plan{Name: name, Revision: strings.Repeat("a", 64)}
	rule := monitor.ResponseRule{ID: "isolate", AgentRef: name, Port: 18088, SourceCIDR: "188.137.254.31/32", Duration: time.Minute}
	assessment := monitor.Assessment{Monitor: name, Revision: plan.Revision, At: time.Now(), State: "down", Zones: []monitor.ZoneAssessment{{Zone: "office", Coverage: "complete", Evidence: []string{"proof"}}}}
	_, err = store.pool.Exec(ctx, `INSERT INTO nmo_resources(kind,name,uid,manifest,effective_revision) VALUES('Monitor',$1,$1,'{}',$2)`, name, plan.Revision)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := store.BeginSession(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Assign(ctx, name, generation, plan.Revision, "office", name); err != nil {
		t.Fatal(err)
	}
	if err = store.Applied(ctx, name, generation, plan.Revision); err != nil {
		t.Fatal(err)
	}
	event, err := store.ReserveResponse(ctx, plan, assessment, rule, generation)
	if err != nil || event == nil {
		t.Fatal(event, err)
	}
	for i := 0; i < 3; i++ {
		duplicate, err := store.ReserveResponse(ctx, plan, assessment, rule, generation)
		if err != nil || duplicate != nil {
			t.Fatal("incident repeated", duplicate, err)
		}
	}
	if err = store.ResponseReceipt(ctx, event.ID, name, generation, "applied", "kernel_timeout_installed"); err != nil {
		t.Fatal(err)
	}
	history, err := store.ResponseHistory(ctx, name)
	if err != nil || len(history) != 1 || history[0].Status != "applied" {
		t.Fatal(history, err)
	}
	if err = store.ResponseReceipt(ctx, event.ID, name, generation, "applied", ""); !errors.Is(err, ErrFenced) {
		t.Fatal("duplicate receipt not fenced", err)
	}
	if err = store.ResetResponses(ctx, plan); err != nil {
		t.Fatal(err)
	}
	next, err := store.ReserveResponse(ctx, plan, assessment, rule, generation)
	if err != nil || next == nil || next.ID == event.ID {
		t.Fatal(next, err)
	}
	newer, err := store.BeginSession(ctx, name)
	if err != nil || newer <= generation {
		t.Fatal(newer, err)
	}
	if err = store.ResponseReceipt(ctx, next.ID, name, generation, "applied", ""); !errors.Is(err, ErrFenced) {
		t.Fatal("stale session receipt accepted", err)
	}
	if _, err = store.ReserveResponse(ctx, plan, assessment, rule, generation); !errors.Is(err, ErrFenced) {
		t.Fatal("stale generation admitted", err)
	}
}
