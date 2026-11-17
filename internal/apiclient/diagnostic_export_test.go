package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDiagnosticExportEvidenceAndFailures(t *testing.T) {
	for _, mode := range []string{"complete", "missing", "forbidden", "cursor-loop", "session-changed", "running"} {
		t.Run(mode, func(t *testing.T) {
			reads := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/diagnostics/run":
					reads++
					state := "completed"
					if mode == "running" {
						state = "running"
					}
					if mode == "session-changed" && reads > 1 {
						state = "cancelled"
					}
					fmt.Fprintf(w, `{"id":"run","monitorId":"web","state":%q,"budgetUsed":{"additionalChecks":1},"evidenceIds":["proof"]}`, state)
				case "/api/v1/diagnostics/run/tasks":
					if mode == "cursor-loop" {
						fmt.Fprint(w, `{"items":[],"nextCursor":"same"}`)
						return
					}
					fmt.Fprint(w, `{"items":[{"id":"task","sessionId":"run","actionId":"http","agentId":"branch","observationIds":["proof"]}],"nextCursor":null}`)
				case "/api/v1/observations/proof":
					if mode == "missing" {
						w.WriteHeader(404)
						fmt.Fprint(w, `{"error":{"code":"NOT_FOUND"}}`)
						return
					}
					if mode == "forbidden" {
						w.WriteHeader(403)
						fmt.Fprint(w, `{"error":{"code":"FORBIDDEN"}}`)
						return
					}
					fmt.Fprint(w, `{"observationId":"proof","monitorId":"web","agentId":"branch","diagnosticSessionId":"run","diagnosticTaskId":"task","values":{"counter":9007199254740993}}`)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			client := &Client{server: server.URL, http: server.Client()}
			raw, err := client.ExportDiagnostic(context.Background(), "run")
			if mode != "complete" && mode != "missing" {
				if err == nil {
					t.Fatal("invalid export accepted", mode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var out DiagnosticExport
			if json.Unmarshal(raw, &out) != nil || out.Schema != 1 || len(out.Tasks) != 1 || out.CollectedAt.IsZero() {
				t.Fatal("invalid package")
			}
			if mode == "complete" && (!out.Complete || len(out.Observations) != 1 || !strings.Contains(string(raw), "9007199254740993")) {
				t.Fatal("evidence/precision lost")
			}
			if mode == "missing" && (out.Complete || len(out.MissingObservationIDs) != 1 || len(out.Observations) != 0) {
				t.Fatal("missing retention evidence hidden")
			}
		})
	}
}
