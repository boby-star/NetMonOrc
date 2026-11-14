package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
)

type DiagnosticStartCommand func(context.Context, auth.AdminCredential, string, diagnosticview.Start) (diagnosticview.Session, error)
type DiagnosticCancelCommand func(context.Context, auth.AdminCredential, string) (diagnosticview.Session, error)

func (h *Handler) SetDiagnosticCommands(start DiagnosticStartCommand, cancel DiagnosticCancelCommand) {
	h.diagnosticStart = start
	h.diagnosticCancel = cancel
}
func (h *Handler) diagnosticCommandRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, done := context.WithTimeout(r.Context(), 5*time.Second)
	defer done()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if principal.Role != "admin" || !slices.Contains(principal.Scopes, "diagnostics:run") {
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
	actor := auth.AdminCredential{UserID: principal.UserID, Hash: auth.HashToken(cookieToken(r))}
	if r.Header.Get("Authorization") != "" {
		raw, _ := bearer(r)
		actor.Hash = auth.HashToken(raw)
		actor.PAT = true
	}
	var view diagnosticview.Session
	if strings.HasPrefix(r.URL.Path, "/api/v1/monitors/") {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/monitors/"), "/")
		if len(parts) != 2 || parts[0] == "" || parts[1] != "diagnostics" || h.diagnosticStart == nil {
			failure(w, 404, "NOT_FOUND", "Diagnostic start unavailable")
			return
		}
		var request diagnosticview.Start
		if !resourceJSON(w, r, &request) {
			return
		}
		view, err = h.diagnosticStart(ctx, actor, parts[0], request)
	} else {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/diagnostics/"), "/")
		if len(parts) != 2 || parts[0] == "" || len(parts[0]) > 128 || parts[1] != "cancel" || h.diagnosticCancel == nil {
			failure(w, 404, "NOT_FOUND", "Diagnostic cancel unavailable")
			return
		}
		if r.Body != nil {
			body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1))
			if e != nil || len(body) > 0 {
				failure(w, 400, "INVALID_ARGUMENT", "Cancel takes no body")
				return
			}
		}
		view, err = h.diagnosticCancel(ctx, actor, parts[0])
	}
	if err != nil {
		resourceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", "/api/v1/diagnostics/"+view.ID)
	w.WriteHeader(202)
	_ = json.NewEncoder(w).Encode(view)
}
