package storage

import (
	"context"
	"errors"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPostgresDurabilityAndFencing(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires real PostgreSQL DSN file")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	s, e := Open(ctx, strings.TrimSpace(string(b)))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	id := "storage-test-" + time.Now().Format("150405.000000000")
	g, e := s.BeginSession(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Applied(ctx, id, g, "revision"); e != nil {
		t.Fatal(e)
	}
	if e = s.Assign(ctx, id, g, "revision", "office", "storage-test"); e != nil {
		t.Fatal(e)
	}
	o := monitor.Observation{ID: id, Agent: id, Zone: "office", Monitor: "storage-test", Revision: "revision", Check: "http", FinishedAt: time.Now().UTC()}
	o.Values = map[string]monitor.Scalar{"large": {Kind: "integer", Integer: 9007199254740993}, "ratio": {Kind: "float", Number: 1}, "huge": {Kind: "float", Number: 1e100}, "flag": {Kind: "boolean", Boolean: false}}
	o.ExtractionStatus = map[string]string{}
	for name := range o.Values {
		o.ExtractionStatus[name] = "success"
	}

	duplicate, e := s.CommitBatch(ctx, id, g, "revision", []monitor.Observation{o})
	if e != nil || duplicate[0] {
		t.Fatalf("initial commit: %v %v", duplicate, e)
	}
	duplicate, e = s.CommitBatch(ctx, id, g, "revision", []monitor.Observation{o})
	if e != nil || !duplicate[0] {
		t.Fatalf("replay: %v %v", duplicate, e)
	}
	stored, e := s.Latest(ctx, monitor.Plan{Name: "storage-test", Revision: "revision", MaxAge: time.Minute}, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, row := range stored {
		if row.ID == o.ID {
			found = true
			for k, v := range o.Values {
				if row.Values[k] != v || row.ExtractionStatus[k] != "success" {
					t.Fatalf("jsonb changed scalar type/value %s: %+v", k, row.Values[k])
				}
			}
		}
	}
	if !found {
		t.Fatal("stored observation missing")
	}
	changed := o
	changed.Outcome = "failure"
	if _, e = s.CommitBatch(ctx, id, g, "revision", []monitor.Observation{changed}); e == nil {
		t.Fatal("conflicting ID silently accepted")
	}
	if _, e = s.BeginSession(ctx, id); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitBatch(ctx, id, g, "revision", []monitor.Observation{o}); !errors.Is(e, ErrFenced) {
		t.Fatalf("stale generation accepted: %v", e)
	}
}
