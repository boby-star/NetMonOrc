package probe

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestExporterErrorIsNotTargetFailure(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer s.Close()
	e := Exporter{endpoint: s.URL}
	c := monitor.Check{ID: "http", URL: "http://127.0.0.1:18080/", Timeout: time.Second}
	o, err := e.HTTP(context.Background(), monitor.Plan{}, c, monitor.Slot{}, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	if err != nil || o.Execution != "execution_failed" || o.Outcome != "unknown" {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestMetricOutcomes(t *testing.T) {
	for _, tc := range []struct {
		body    string
		success bool
		valid   bool
	}{
		{"probe_success 1\nprobe_http_status_code 200\n", true, true},
		{"probe_success 0\nprobe_http_status_code 503\n", false, true},
		{"probe_success NaN\n", false, false},
		{"probe_success 1\nprobe_success 0\n", false, false},
		{"# HELP probe_success description\n", false, false},
	} {
		success, _, e := parseMetrics(tc.body)
		if (e == nil) != tc.valid || (e == nil && success != tc.success) {
			t.Fatalf("%q: %v %v", tc.body, success, e)
		}
	}
}
