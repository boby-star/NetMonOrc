package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/alertview"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

type AlertAcknowledgeCommand func(context.Context, auth.AdminCredential, string, string, alertview.Acknowledge) (alertview.Alert, error)

func (h *Handler) SetAlertAcknowledge(command AlertAcknowledgeCommand) { h.alertAcknowledge = command }
func alertResponse(w http.ResponseWriter, v alertview.Alert) {
	w.Header().Set("ETag", `"`+alertview.Version(v)+`"`)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) acknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if principal.Role != "admin" || !slices.Contains(principal.Scopes, "alerts:manage") {
		authError(w, auth.ErrForbidden)
		return
	}
	if !sameOrigin(r) {
		failure(w, 403, "ORIGIN_REJECTED", "Same-origin required")
		return
	}
	if r.Header.Get("Authorization") == "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(principal.CSRFToken)) != 1 {
		failure(w, 403, "CSRF_REJECTED", "Valid CSRF token required")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/alerts/"), "/")
	if len(parts) != 2 || parts[0] == "" || len(parts[0]) > 128 || parts[1] != "acknowledge" || h.alertAcknowledge == nil {
		failure(w, 404, "NOT_FOUND", "Alert acknowledgement unavailable")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	expected, ok := resourceMatch(w, r)
	if !ok {
		return
	}
	var request *alertview.Acknowledge
	if !resourceJSON(w, r, &request) {
		return
	}
	if request == nil {
		failure(w, 400, "INVALID_ARGUMENT", "Object required")
		return
	}
	actor := auth.AdminCredential{UserID: principal.UserID, Hash: auth.HashToken(cookieToken(r))}
	if r.Header.Get("Authorization") != "" {
		raw, _ := bearer(r)
		actor.Hash = auth.HashToken(raw)
		actor.PAT = true
	}
	view, err := h.alertAcknowledge(ctx, actor, parts[0], expected, *request)
	if err != nil {
		if errors.Is(err, resources.ErrConflict) {
			failure(w, 409, "ACKNOWLEDGEMENT_CONFLICT", "Alert is inactive or already acknowledged with a different note")
			return
		}
		if errors.Is(err, resources.ErrInvalid) {
			failure(w, 422, "INVALID_ARGUMENT", "Note requires at most 1024 Unicode characters and no NUL")
			return
		}
		resourceError(w, err)
		return
	}
	alertResponse(w, view)
}
