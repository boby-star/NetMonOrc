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

func TestPostgresIndependentRevisionAcknowledgements(t *testing.T) {
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
	store, err := Open(ctx, strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	agent := "multi-" + newID()
	generation, err := store.BeginSession(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}
	var observations []monitor.Observation
	for _, name := range []string{"first", "second"} {
		if err = store.Assign(ctx, agent, generation, name, "office", name); err != nil {
			t.Fatal(err)
		}
		if err = store.Applied(ctx, agent, generation, name); err != nil {
			t.Fatal(err)
		}
		observations = append(observations, monitor.Observation{ID: newID(), Agent: agent, Zone: "office", Monitor: name, Revision: name, Check: "same_check", FinishedAt: time.Now().UTC()})
	}
	// The last ACK must not invalidate the first plan's commit rights.
	for i, name := range []string{"first", "second"} {
		if _, err = store.CommitBatch(ctx, agent, generation, name, observations[i:i+1]); err != nil {
			t.Fatal(name, err)
		}
	}
	duplicate, err := store.CommitBatch(ctx, agent, generation, "second", observations)
	if err != nil || len(duplicate) != 2 || !duplicate[0] || !duplicate[1] {
		t.Fatal("mixed replay lost idempotency", duplicate, err)
	}
	newer, err := store.BeginSession(ctx, agent)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Applied(ctx, agent, newer, "second"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CommitBatch(ctx, agent, newer, "first", observations); !errors.Is(err, ErrFenced) {
		t.Fatal("previous generation ACK authorized new session", err)
	}
	if _, err = store.CommitBatch(ctx, agent, generation, "second", observations); !errors.Is(err, ErrFenced) {
		t.Fatal("old stream retained write rights", err)
	}
	if err = store.Applied(ctx, agent, newer, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CommitBatch(ctx, agent, newer, "first", observations); err != nil {
		t.Fatal("independent ACK recovery", err)
	}
}
