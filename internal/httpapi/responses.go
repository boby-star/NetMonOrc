package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/boby-star/NetMonOrc/internal/responseview"
)

type ResponseLoad func(context.Context, string) ([]responseview.Event, error)

func (h *Handler) SetResponses(load ResponseLoad) { h.responses = load }
func (h *Handler) responseHistory(w http.ResponseWriter, r *http.Request) {
	if !h.authorize(w, r) {
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "Read-only endpoint")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	if h.responses == nil {
		failure(w, 503, "RESPONSES_UNAVAILABLE", "Response history unavailable")
		return
	}
	select {
	case h.readSlots <- struct{}{}:
		defer func() { <-h.readSlots }()
	default:
		failure(w, 429, "API_BUSY", "Reader busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	rows, err := h.responses(ctx, h.monitor)
	if err != nil {
		failure(w, 503, "RESPONSES_UNAVAILABLE", "Response history unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
}
