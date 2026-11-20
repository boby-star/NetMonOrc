package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestExternalComponentRunsExactPreparedModuleWithoutFallback(t *testing.T) {
	c := monitor.Check{ID: "http", Type: "http", Runner: "blackbox", URL: "http://127.0.0.1:1234/", Method: "GET", ExpectedStatus: []int{200}, MaxBodyBytes: 16384, Timeout: time.Second}
	p := monitor.Plan{Name: "external", Revision: "revision", Checks: []monitor.Check{c}}
	module, _, err := externalModule(p, c)
	if err != nil {
		t.Fatal(err)
	}
	same := c
	same.ID = "diagnostic-other-id"
	other, _, _ := externalModule(p, same)
	if module != other {
		t.Fatal("check ID changed identical module")
	}
	same.ExpectedStatus = []int{503}
	other, _, _ = externalModule(p, same)
	if module == other {
		t.Fatal("probe semantics missing from module identity")
	}
	b, err := ExternalModules([]monitor.Plan{p, p})
	var cfg struct {
		Modules map[string]any `json:"modules"`
	}
	if err != nil || json.Unmarshal(b, &cfg) != nil || len(cfg.Modules) != 1 || cfg.Modules[module] == nil {
		t.Fatal("invalid deterministic exporter config", err)
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/probe" || r.URL.Query().Get("module") != module || r.URL.Query().Get("target") != c.URL {
			t.Error("wrong external module/target")
		}
		fmt.Fprintln(w, "probe_success 1\nprobe_http_status_code 200")
	}))
	p.Components = map[string]monitor.ComponentBinding{"branch": {Enabled: true, Endpoint: server.URL, ModuleSet: "standard-network"}}
	e, err := StartAgentExporter(context.Background(), "must-not-execute", p, "branch")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	allowed := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	o, err := e.Run(context.Background(), p, c, monitor.Slot{Agent: "branch", Zone: "office"}, allowed)
	if err != nil || o.Execution != "completed" || o.Outcome != "success" || calls != 1 {
		t.Fatal(o, err)
	}
	server.Close()
	o, err = e.Run(context.Background(), p, c, monitor.Slot{Agent: "branch"}, allowed)
	if err != nil || o.Execution != "execution_failed" || o.Outcome != "unknown" || o.Reason != "runner_unavailable" {
		t.Fatal("exporter outage became target failure", o, err)
	}
	binding := p.Components["branch"]
	binding.Enabled = false
	p.Components["branch"] = binding
	if _, err = StartAgentExporter(context.Background(), "must-not-execute", p, "branch"); !errors.Is(err, ErrRunnerUnavailable) {
		t.Fatal("disabled binding silently fell back", err)
	}
	if _, err = dialLocalExporter(context.Background(), "tcp", "169.254.169.254:80"); err == nil {
		t.Fatal("metadata endpoint admitted")
	}
}
