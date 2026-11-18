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

func TestDiagnosticExportRegularBaselineProvenance(t *testing.T) {
	for _, mode := range []string{"complete", "missing", "foreign-monitor", "foreign-revision", "foreign-agent", "foreign-check", "foreign-zone", "foreign-time", "foreign-outcome", "diagnostic-instead-of-regular", "forbidden"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/diagnostics/run":
					fmt.Fprint(w, `{"id":"run","monitorId":"web","configRevision":"r1","state":"completed","evidenceIds":[],"explanation":{"decisions":[],"baselineEvidence":[{"observationId":"regular","actionId":"http","checkId":"regular-http","agentId":"branch","zone":"office","configRevision":"r1","finishedAt":"2026-11-18T12:00:00Z","validUntil":"2026-11-18T12:00:20Z","executionStatus":"completed","probeOutcome":"success"},{"observationId":"regular","actionId":"same-probe-alias","checkId":"regular-http","agentId":"branch","zone":"office","configRevision":"r1","finishedAt":"2026-11-18T12:00:00Z","validUntil":"2026-11-18T12:00:20Z","executionStatus":"completed","probeOutcome":"success"}]}}`)
				case "/api/v1/diagnostics/run/tasks":
					fmt.Fprint(w, `{"items":[],"nextCursor":null}`)
				case "/api/v1/observations/regular":
					if mode == "missing" {
						w.WriteHeader(404)
						return
					}
					if mode == "forbidden" {
						w.WriteHeader(403)
						return
					}
					o := map[string]string{"observationId": "regular", "monitorId": "web", "checkId": "regular-http", "agentId": "branch", "networkZone": "office", "configRevision": "r1", "finishedAt": "2026-11-18T12:00:00Z", "executionStatus": "completed", "probeOutcome": "success"}
					fields := map[string]string{"foreign-monitor": "monitorId", "foreign-revision": "configRevision", "foreign-agent": "agentId", "foreign-check": "checkId", "foreign-zone": "networkZone", "foreign-time": "finishedAt", "foreign-outcome": "probeOutcome", "diagnostic-instead-of-regular": "diagnosticSessionId"}
					if field := fields[mode]; field != "" {
						o[field] = "foreign"
					}
					_ = json.NewEncoder(w).Encode(o)
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
					t.Fatal("invalid baseline accepted", mode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var out DiagnosticExport
			if json.Unmarshal(raw, &out) != nil || !out.BaselineEvidenceKnown || len(out.Tasks) != 0 {
				t.Fatal("baseline-only package lost", string(raw))
			}
			if mode == "complete" && (!out.Complete || len(out.Observations) != 1) {
				t.Fatal("shared physical baseline not deduplicated")
			}
			if mode == "missing" && (out.Complete || len(out.MissingObservationIDs) != 1) {
				t.Fatal("missing baseline hidden")
			}
		})
	}
}
