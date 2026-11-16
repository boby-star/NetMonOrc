package httpapi

import (
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/dashboard"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"net/http"
	"slices"
	"time"
)

type DashboardLoad func(context.Context, dashboard.Reads) (dashboard.Source, error)

func (h *Handler) SetDashboard(load DashboardLoad) { h.dashboard = load }
func currentAssessment(a monitor.Assessment, found bool, name, revision string) bool {
	age := time.Since(a.At)
	return found && a.Monitor == name && age <= 5*time.Second && age >= -5*time.Second && (revision == "" || a.Revision == revision)
}
func (h *Handler) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	has := func(scope string) bool { return slices.Contains(principal.Scopes, scope) }
	reads := dashboard.Reads{Resources: has("resources:read"), Agents: has("agents:read"), Alerts: has("alerts:read"), Diagnostics: has("diagnostics:read")}
	if !reads.Resources && !reads.Agents && !reads.Alerts && !reads.Diagnostics {
		authError(w, auth.ErrForbidden)
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "GET required")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	if h.dashboard == nil {
		failure(w, 404, "NOT_FOUND", "Dashboard unavailable")
		return
	}
	select {
	case h.readSlots <- struct{}{}:
		defer func() { <-h.readSlots }()
	default:
		authError(w, auth.ErrBusy)
		return
	}
	source, err := h.dashboard(ctx, reads)
	if err != nil {
		failure(w, 503, "DASHBOARD_UNAVAILABLE", "Dashboard temporarily unavailable")
		return
	}
	v := dashboard.Summary{ObservedAt: time.Now().UTC()}
	if reads.Resources {
		v.MonitorStates = map[string]int{"healthy": 0, "degraded": 0, "down": 0, "unknown": source.Monitors}
		v.CoverageStates = map[string]int{"sufficient": 0, "degraded": 0, "unknown": source.Monitors}
		assessments := source.Assessments
		if assessments == nil && source.Assessment != nil {
			assessments = []monitor.Assessment{*source.Assessment}
		}
		seen := map[string]bool{}
		for _, assessment := range assessments {
			revision := h.expectedRevision
			if h.monitorRevisions != nil {
				var found bool
				revision, found = h.monitorRevisions[assessment.Monitor]
				if !found {
					continue
				}
			} else if assessment.Monitor != h.monitor {
				continue
			}
			if seen[assessment.Monitor] || !currentAssessment(assessment, true, assessment.Monitor, revision) || len(seen) >= source.Monitors {
				continue
			}
			seen[assessment.Monitor] = true
			a := projectAssessment(assessment)
			state := a.State
			if !slices.Contains([]string{"healthy", "degraded", "down", "unknown"}, state) {
				state = "unknown"
			}
			v.MonitorStates["unknown"]--
			v.MonitorStates[state]++
			v.CoverageStates["unknown"]--
			v.CoverageStates[a.CoverageState]++
		}
	}
	if reads.Agents && h.agents != nil {
		v.AgentCounts = map[string]int{"online": 0, "offline": 0, "degraded": 0, "revoked": 0}
		for _, agent := range h.agents() {
			v.AgentCounts[agent.OnlineState]++
		}
	}
	if reads.Alerts {
		v.FiringAlerts = &source.Firing
	}
	if reads.Diagnostics {
		v.RecentDiagnostics = &source.Diagnostics
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
