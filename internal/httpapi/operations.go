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

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func (h *Handler) operationRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if !slices.Contains(principal.Scopes, "resources:read") {
		authError(w, auth.ErrForbidden)
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "GET required")
		return
	}
	if h.resources == nil {
		failure(w, 404, "NOT_FOUND", "Operations unavailable")
		return
	}
	if !h.resources.Admit() {
		authError(w, auth.ErrBusy)
		return
	}
	defer h.resources.Release()
	if len(r.URL.RawQuery) > 4096 {
		failure(w, 400, "INVALID_ARGUMENT", "Query too long")
		return
	}
	if r.URL.Path != "/api/v1/operations" {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/operations/")
		if id == "" || len(id) > 128 || strings.Contains(id, "/") {
			failure(w, 404, "NOT_FOUND", "Operation unavailable")
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
			return
		}
		op, found, e := h.resources.Repo.Operation(ctx, id)
		if e != nil {
			resourceError(w, e)
			return
		}
		if !found {
			resourceError(w, resources.ErrNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(op)
		return
	}
	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid query")
		return
	}
	for key, values := range params {
		if len(values) != 1 || !slices.Contains([]string{"limit", "cursor", "state"}, key) {
			failure(w, 400, "INVALID_ARGUMENT", "Invalid query parameters")
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
	if state != "" && !slices.Contains([]string{"accepted", "running", "completed", "failed"}, state) {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid operation state")
		return
	}
	if len(params.Get("cursor")) > 128 {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid cursor")
		return
	}
	rows, e := h.resources.Repo.Operations(ctx, params.Get("cursor"), state, limit)
	if e != nil {
		resourceError(w, e)
		return
	}
	page := resources.OperationPage{Items: rows}
	if len(rows) > limit {
		cursor := rows[limit-1].ID
		page.NextCursor = &cursor
		page.Items = rows[:limit]
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}
