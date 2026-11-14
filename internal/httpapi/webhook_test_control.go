package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/operationview"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

type WebhookTestCommand func(context.Context, auth.AdminCredential) (operationview.Operation, error)

func (h *Handler) SetWebhookTest(command WebhookTestCommand) { h.webhookTest = command }
func (h *Handler) testWebhook(w http.ResponseWriter, r *http.Request) {
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
	if r.Method != "POST" {
		w.Header().Set("Allow", "POST")
		failure(w, 405, "METHOD_NOT_ALLOWED", "POST required")
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
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	var request *struct{}
	if !resourceJSON(w, r, &request) {
		return
	}
	if request == nil {
		failure(w, 400, "INVALID_ARGUMENT", "Empty object required")
		return
	}
	if h.webhookTest == nil {
		failure(w, 409, "WEBHOOK_TEST_UNAVAILABLE", "Webhook sender unavailable")
		return
	}
	actor := auth.AdminCredential{UserID: principal.UserID, Hash: auth.HashToken(cookieToken(r))}
	if r.Header.Get("Authorization") != "" {
		raw, _ := bearer(r)
		actor.Hash = auth.HashToken(raw)
		actor.PAT = true
	}
	op, err := h.webhookTest(ctx, actor)
	if errors.Is(err, resources.ErrConflict) {
		failure(w, 409, "WEBHOOK_TEST_UNAVAILABLE", "Webhook disabled or another test is pending")
		return
	}
	if err != nil {
		resourceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/v1/operations/"+op.ID)
	w.WriteHeader(202)
	_ = json.NewEncoder(w).Encode(op)
}
