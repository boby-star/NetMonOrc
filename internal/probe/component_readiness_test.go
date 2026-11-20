package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestComponentHealthIsBoundedLocalAndDoesNotProbeTargets(t *testing.T) {
	for _, mode := range []string{"ready", "redirect", "oversize", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/-/healthy" || r.URL.RawQuery != "" {
					t.Error("readiness invoked a target probe")
				}
				switch mode {
				case "redirect":
					http.Redirect(w, r, "http://169.254.169.254/", http.StatusFound)
				case "oversize":
					fmt.Fprint(w, strings.Repeat("x", 4097))
				case "unavailable":
					w.WriteHeader(503)
				default:
					fmt.Fprint(w, "ok")
				}
			}))
			defer server.Close()
			b := monitor.ComponentBinding{Endpoint: server.URL, ModuleSet: "standard-network", Enabled: true}
			if ComponentReady(context.Background(), b) != (mode == "ready") {
				t.Fatal("incorrect health result")
			}
		})
	}
}
