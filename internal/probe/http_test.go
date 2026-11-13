package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestBoundedHTTP(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bad":
			w.WriteHeader(503)
		case "/redirect":
			w.Header().Set("Location", "/")
			w.WriteHeader(302)
		case "/large":
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte("0123456789012345"))
		default:
			w.WriteHeader(200)
		}
	}))
	defer s.Close()
	p := monitor.Plan{Name: "web", Revision: "rev"}
	slot := monitor.Slot{Agent: "a", Zone: "office"}
	allow := []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}
	for _, tc := range []struct{ path, outcome, execution string }{{"/", "success", "completed"}, {"/bad", "failure", "completed"}, {"/redirect", "failure", "completed"}, {"/large", "unknown", "execution_failed"}} {
		c := monitor.Check{ID: "http", URL: s.URL + tc.path, Method: "GET", ExpectedStatus: []int{200}, Timeout: time.Second, MaxBodyBytes: 8}
		o, e := HTTP(context.Background(), p, c, slot, allow)
		if e != nil {
			t.Fatal(e)
		}
		if o.Outcome != tc.outcome || o.Execution != tc.execution {
			t.Fatalf("%s: %+v", tc.path, o)
		}
	}
	_, e := HTTP(context.Background(), p, monitor.Check{URL: s.URL}, slot, nil)
	if e == nil {
		t.Fatal("accepted target without allowlist")
	}
	_, e = HTTP(context.Background(), p, monitor.Check{URL: "http://169.254.169.254/"}, slot, []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")})
	if e == nil {
		t.Fatal("accepted metadata target")
	}
}
