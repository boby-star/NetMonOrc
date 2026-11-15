package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestIdleAgentDoesNotRunRegularChecks(t *testing.T) {
	var selected, idle atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/branch" {
			selected.Add(1)
		} else {
			idle.Add(1)
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	boot, err := bootTime()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	done := make(chan error, 2)
	for _, id := range []string{"branch", "dc"} {
		spool, err := OpenSpool(t.TempDir(), id)
		if err != nil {
			t.Fatal(err)
		}
		defer spool.Close()
		p := monitor.Plan{Name: "test", Revision: id, Checks: []monitor.Check{{ID: "http", Type: "http", Runner: "native-http", URL: server.URL + "/" + id, Method: "GET", ExpectedStatus: []int{200}, MaxBodyBytes: 1024, Interval: 10 * time.Second, Timeout: time.Second}}, FixedPlacement: &monitor.FixedPlacement{Selected: []monitor.Slot{{Agent: "branch", Zone: "office"}}}}
		st := &state{}
		st.set(&assignment{plan: p, zone: "office", deadline: time.Now().Add(5 * time.Second), bootDeadline: boot + 5*time.Second})
		go func(id string, st *state, spool *Spool) {
			done <- schedule(ctx, st, spool, id, "", []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")})
		}(id, st, spool)
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if selected.Load() == 0 || idle.Load() != 0 {
		t.Fatal("regular selection not enforced", selected.Load(), idle.Load())
	}
}
