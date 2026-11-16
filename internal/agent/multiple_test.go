package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestTwoMonitorPlansKeepIndependentChecks(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/negative" {
			w.WriteHeader(503)
		} else {
			w.WriteHeader(200)
		}
	}))
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
	for _, name := range []string{"positive", "negative"} {
		p := monitor.Plan{Name: name, Revision: name, Checks: []monitor.Check{{ID: "same_check", Type: "http", Runner: "native-http", URL: target.URL + "/" + name, Method: "GET", ExpectedStatus: []int{200}, MaxBodyBytes: 1024, Interval: time.Second, Timeout: time.Second}}}
		st.set(&assignment{plan: p, zone: "office", deadline: time.Now().Add(5 * time.Second), bootDeadline: boot + 5*time.Second})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2200*time.Millisecond)
	defer cancel()
	if err = schedule(ctx, st, spool, "branch", "", []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, o := range spool.Pending() {
		counts[o.Monitor]++
		want := "success"
		if o.Monitor == "negative" {
			want = "failure"
		}
		if o.Execution != "completed" || o.Outcome != want || o.Check != "same_check" || o.Revision != o.Monitor {
			t.Fatalf("mixed observation: %+v", o)
		}
	}
	if counts["positive"] < 2 || counts["negative"] < 2 {
		t.Fatal("one plan displaced another or shared its interval", counts)
	}
}
