package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestDiskSpoolBackpressureAndResume(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	defer server.Close()
	dir := t.TempDir()
	spool, err := OpenSpool(dir, "branch")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if spool != nil {
			spool.Close()
		}
	}()
	old := time.Now().Add(-time.Hour)
	for i := 0; i < MaxPending; i++ {
		o := monitor.Observation{ID: fmt.Sprintf("%032x", i), Agent: "branch", Monitor: "test", Revision: "r", Check: "http", Zone: "office", StartedAt: old, FinishedAt: old, Execution: "completed", Outcome: "success"}
		if err := spool.Add(o); err != nil {
			t.Fatal(err)
		}
	}
	boot, err := bootTime()
	if err != nil {
		t.Fatal(err)
	}
	st := &state{current: &assignment{plan: monitor.Plan{Name: "test", Revision: "r", Checks: []monitor.Check{{ID: "http", Type: "http", Runner: "native-http", URL: server.URL, Method: "GET", Timeout: time.Second, Interval: time.Hour, ExpectedStatus: []int{200}, MaxBodyBytes: 1024}}}, zone: "office", deadline: time.Now().Add(10 * time.Second), bootDeadline: boot + 10*time.Second}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- schedule(ctx, st, spool, "branch", "", []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	}()
	time.Sleep(600 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("full disk queue allowed a probe")
	}
	ids := []string{fmt.Sprintf("%032x", 0)}
	if err := spool.Ack(ids); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) && calls.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Fatalf("probe did not resume: %d", calls.Load())
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !spool.Full() {
		t.Fatal("resumed result not persisted into the free slot")
	}
	if _, err := os.Stat(dir + "/" + ids[0] + ".json"); !os.IsNotExist(err) {
		t.Fatal("ACK did not remove the acknowledged file")
	}
	if err := spool.Close(); err != nil {
		t.Fatal(err)
	}
	spool = nil
	reopened, err := OpenSpool(dir, "branch")
	if err != nil {
		t.Fatal(err)
	}
	spool = reopened
	if !spool.Full() {
		t.Fatal("restart lost pending records")
	}
	total := 0
	for spool.Full() || len(spool.Pending()) > 0 {
		batch := spool.Pending()
		if len(batch) == 0 {
			t.Fatal("queue cannot drain")
		}
		ids = nil
		for _, o := range batch {
			ids = append(ids, o.ID)
		}
		if err := spool.Ack(ids); err != nil {
			t.Fatal(err)
		}
		total += len(ids)
	}
	if total != MaxPending {
		t.Fatalf("lost or duplicated records: %d", total)
	}
}
