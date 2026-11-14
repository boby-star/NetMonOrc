package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"github.com/boby-star/NetMonOrc/internal/auth"
)

func (h *Handler) SetAgents(load func() []agentstatus.View) { h.agents = load }
func (h *Handler) agentRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if !slices.Contains(principal.Scopes, "agents:read") {
		authError(w, auth.ErrForbidden)
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "Agent snapshot is read-only")
		return
	}
	if h.agents == nil || r.URL.Path == "/api/v1/agents/enrollments" || strings.HasPrefix(r.URL.Path, "/api/v1/agents/enrollments/") {
		failure(w, 404, "NOT_FOUND", "Agent operation unavailable")
		return
	}
	rows := h.agents()
	if r.URL.Path != "/api/v1/agents" {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
		if id == "" || strings.Contains(id, "/") || r.URL.RawQuery != "" {
			failure(w, 404, "NOT_FOUND", "Agent unavailable")
			return
		}
		for _, v := range rows {
			if v.AgentID == id {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(v)
				return
			}
		}
		failure(w, 404, "NOT_FOUND", "Agent unavailable")
		return
	}
	if len(r.URL.RawQuery) > 4096 {
		failure(w, 400, "INVALID_ARGUMENT", "Query too long")
		return
	}
	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid query")
		return
	}
	for key, values := range params {
		if len(values) != 1 || !slices.Contains([]string{"limit", "cursor", "zone", "state"}, key) {
			failure(w, 400, "INVALID_ARGUMENT", "Unsupported or duplicate filter")
			return
		}
	}
	limit := 50
	if raw, ok := params["limit"]; ok {
		limit, err = strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > 200 {
			failure(w, 400, "INVALID_ARGUMENT", "limit requires 1..200")
			return
		}
	}
	state := params.Get("state")
	if state != "" && !slices.Contains([]string{"online", "offline", "degraded", "revoked"}, state) {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid state filter")
		return
	}
	page := agentstatus.Page{Items: []agentstatus.View{}}
	for _, v := range rows {
		if v.AgentID <= params.Get("cursor") || (state != "" && v.OnlineState != state) || (params.Get("zone") != "" && v.NetworkZone != params.Get("zone")) {
			continue
		}
		page.Items = append(page.Items, v)
	}
	if len(page.Items) > limit {
		cursor := page.Items[limit-1].AgentID
		page.NextCursor = &cursor
		page.Items = page.Items[:limit]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}
