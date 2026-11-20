package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/fallback"
)

func (h *Handler) SetFallbacks(reader fallback.Reader) { h.fallbacks = reader }
func (h *Handler) fallbackRoutes(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeRead(w, r, false) {
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "Read-only fallback history")
		return
	}
	if h.fallbacks == nil {
		failure(w, 404, "NOT_FOUND", "Fallback history unavailable")
		return
	}
	q, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(r.URL.RawQuery) > 4096 {
		failure(w, 400, "INVALID_ARGUMENT", "Malformed fallback query")
		return
	}
	limit := 50
	for key, values := range q {
		if len(values) != 1 || (key != "monitor" && key != "cursor" && key != "limit") {
			failure(w, 400, "INVALID_ARGUMENT", "Unknown or duplicate fallback filter")
			return
		}
	}
	if q.Has("limit") {
		limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			failure(w, 400, "INVALID_ARGUMENT", "limit requires 1..100")
			return
		}
	}
	if q.Get("monitor") != "" && !resourceNamePattern.MatchString(q.Get("monitor")) || q.Get("cursor") != "" && !fallbackIDPattern.MatchString(q.Get("cursor")) {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid fallback filter")
		return
	}
	select {
	case h.readSlots <- struct{}{}:
		defer func() { <-h.readSlots }()
	default:
		failure(w, 429, "API_BUSY", "Fallback reader busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if r.URL.Path != "/api/v1/fallback-runs" {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/fallback-runs/")
		if !fallbackIDPattern.MatchString(id) || len(q) != 0 {
			failure(w, 400, "INVALID_ARGUMENT", "Invalid fallback ID/query")
			return
		}
		run, found, err := h.fallbacks.ReadFallback(ctx, id)
		if err != nil {
			failure(w, 503, "FALLBACK_UNAVAILABLE", "Fallback history temporarily unavailable")
			return
		}
		if !found {
			failure(w, 404, "NOT_FOUND", "Fallback run not found")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(run)
		return
	}
	rows, err := h.fallbacks.FallbackRuns(ctx, q.Get("monitor"), q.Get("cursor"), limit)
	if err != nil {
		failure(w, 503, "FALLBACK_UNAVAILABLE", "Fallback history temporarily unavailable")
		return
	}
	var next *string
	if len(rows) > limit {
		cursor := rows[limit-1].ID
		next = &cursor
		rows = rows[:limit]
	}
	if rows == nil {
		rows = []fallback.Run{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Items      []fallback.Run `json:"items"`
		NextCursor *string        `json:"nextCursor"`
	}{rows, next})
}

var fallbackIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
