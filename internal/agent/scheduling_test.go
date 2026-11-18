package agent

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestRegularDispatchPriorityAndDisabled(t *testing.T) {
	requests := make(chan string, 10)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests <- r.URL.Path; w.WriteHeader(200) }))
	defer target.Close()
	spool, err := OpenSpool(t.TempDir(), "branch")
	if err != nil {
		t.Fatal(err)
	}
	defer spool.Close()
	boot, err := bootTime()
	if err != nil {
		t.Fatal(err)
	}
	st := &state{}
	for _, v := range []struct {
		name     string
		priority int
		disabled bool
	}{{"a-low", 10, false}, {"z-high", 90, false}, {"disabled", 100, true}} {
		p := monitor.Plan{Name: v.name, Revision: v.name, Disabled: v.disabled, Checks: []monitor.Check{{ID: "http", Type: "http", Runner: "native-http", Priority: v.priority, URL: target.URL + "/" + v.name, Method: "GET", ExpectedStatus: []int{200}, MaxBodyBytes: 1024, Interval: time.Minute, Timeout: time.Second}}}
		st.set(&assignment{plan: p, zone: "office", deadline: time.Now().Add(5 * time.Second), bootDeadline: boot + 5*time.Second})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	if err = schedule(ctx, st, spool, "branch", "", []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 {
		t.Fatalf("disabled work ran or active work missing: %d", len(requests))
	}
	if first, second := <-requests, <-requests; first != "/z-high" || second != "/a-low" {
		t.Fatal("dispatch priority ignored", first, second)
	}
}
func TestScheduleJitterStableBoundedAndSourceSpecific(t *testing.T) {
	limit := 2 * time.Second
	first := scheduleJitter("branch", "rev/http", 7, limit)
	if first < 0 || first > limit || first != scheduleJitter("branch", "rev/http", 7, limit) {
		t.Fatal("unstable jitter")
	}
	if first == scheduleJitter("dc", "rev/http", 7, limit) || first == scheduleJitter("branch", "rev/http", 8, limit) {
		t.Fatal("jitter ignores source or ordinal")
	}
	if scheduleJitter("branch", "rev/http", 7, 0) != 0 {
		t.Fatal("default adds hidden jitter")
	}
}
