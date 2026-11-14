package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
)

const cookieName = "netmonorc_session"

func cookieToken(r *http.Request) string {
	raw := ""
	count := 0
	for _, c := range r.Cookies() {
		if c.Name == cookieName {
			count++
			if count > 1 {
				return ""
			}
			raw = c.Value
		}
	}
	return raw
}
func sameOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	return origin == "" || origin == "https://"+r.Host
}
func authError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInput):
		failure(w, 422, "INVALID_ARGUMENT", "Invalid token request")
	case errors.Is(err, auth.ErrForbidden):
		failure(w, 403, "FORBIDDEN", "Requested permission unavailable")
	case errors.Is(err, auth.ErrNotFound):
		failure(w, 404, "NOT_FOUND", "Token not found")
	case errors.Is(err, auth.ErrPassword):
		failure(w, 422, "INVALID_PASSWORD", "Password requires 16..256 UTF-8 bytes")
	case errors.Is(err, auth.ErrCredentials):
		failure(w, 401, "UNAUTHORIZED", "Invalid credentials or session")
	case errors.Is(err, auth.ErrBusy):
		w.Header().Set("Retry-After", "60")
		failure(w, 429, "AUTH_BUSY", "Try authentication later")
	default:
		failure(w, 503, "AUTH_UNAVAILABLE", "Authentication temporarily unavailable")
	}
}
func (h *Handler) authRoutes(w http.ResponseWriter, r *http.Request) {
	if h.auth == nil {
		failure(w, 404, "NOT_FOUND", "Session authentication unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if r.URL.Path == "/api/v1/auth/tokens" || strings.HasPrefix(r.URL.Path, "/api/v1/auth/tokens/") {
		h.tokenRoutes(w, r)
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	switch r.URL.Path {
	case "/api/v1/auth/login":
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			failure(w, 405, "METHOD_NOT_ALLOWED", "POST required")
			return
		}
		if !sameOrigin(r) {
			failure(w, 403, "ORIGIN_REJECTED", "Same-origin request required")
			return
		}
		kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || kind != "application/json" {
			failure(w, 400, "INVALID_ARGUMENT", "JSON login required")
			return
		}
		var input struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&input); err != nil {
			failure(w, 400, "INVALID_ARGUMENT", "Invalid login request")
			return
		}
		if decoder.Decode(new(any)) != io.EOF {
			failure(w, 400, "INVALID_ARGUMENT", "One login request required")
			return
		}
		view, raw, err := h.auth.Login(ctx, input.Username, input.Password)
		input.Password = ""
		if err != nil {
			authError(w, err)
			return
		}
		if old := cookieToken(r); old != "" {
			if err = h.auth.Logout(ctx, old); err != nil {
				authError(w, err)
				return
			}
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: raw, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: *view.ExpiresAt, MaxAge: int(auth.SessionTTL.Seconds())})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	case "/api/v1/auth/me":
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			failure(w, 405, "METHOD_NOT_ALLOWED", "GET required")
			return
		}
		view, err := h.principal(ctx, r)
		if err != nil {
			authError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	case "/api/v1/auth/change-password":
		if r.Header.Get("Authorization") != "" {
			authError(w, auth.ErrCredentials)
			return
		}
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			failure(w, 405, "METHOD_NOT_ALLOWED", "POST required")
			return
		}
		if !sameOrigin(r) {
			failure(w, 403, "ORIGIN_REJECTED", "Same-origin request required")
			return
		}
		raw := cookieToken(r)
		current, err := h.auth.Me(ctx, raw)
		if err != nil {
			authError(w, err)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(current.CSRFToken)) != 1 {
			failure(w, 403, "CSRF_REJECTED", "Valid CSRF token required")
			return
		}
		kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || kind != "application/json" {
			failure(w, 400, "INVALID_ARGUMENT", "JSON password change required")
			return
		}
		var input struct {
			CurrentPassword string `json:"currentPassword"`
			NewPassword     string `json:"newPassword"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&input); err != nil {
			failure(w, 400, "INVALID_ARGUMENT", "Invalid password change request")
			return
		}
		if decoder.Decode(new(any)) != io.EOF {
			failure(w, 400, "INVALID_ARGUMENT", "One password change required")
			return
		}
		view, fresh, err := h.auth.ChangePassword(ctx, raw, input.CurrentPassword, input.NewPassword)
		input.CurrentPassword, input.NewPassword = "", ""
		if err != nil {
			authError(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: fresh, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: *view.ExpiresAt, MaxAge: int(auth.SessionTTL.Seconds())})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	case "/api/v1/auth/logout":
		if r.Header.Get("Authorization") != "" {
			authError(w, auth.ErrCredentials)
			return
		}
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			failure(w, 405, "METHOD_NOT_ALLOWED", "POST required")
			return
		}
		if !sameOrigin(r) {
			failure(w, 403, "ORIGIN_REJECTED", "Same-origin request required")
			return
		}
		raw := cookieToken(r)
		view, err := h.auth.Me(ctx, raw)
		if err != nil {
			authError(w, err)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(view.CSRFToken)) != 1 {
			failure(w, 403, "CSRF_REJECTED", "Valid CSRF token required")
			return
		}
		if err = h.auth.Logout(ctx, raw); err != nil {
			authError(w, err)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: time.Unix(1, 0), MaxAge: -1})
		w.WriteHeader(204)
	default:
		failure(w, 404, "NOT_FOUND", "Auth endpoint not implemented")
	}
}
func bearer(r *http.Request) (string, error) {
	scheme, value, present := strings.Cut(r.Header.Get("Authorization"), " ")
	if !present || !strings.EqualFold(scheme, "Bearer") || strings.ContainsAny(value, " \t\r\n") {
		return "", auth.ErrCredentials
	}
	return value, nil
}
func (h *Handler) principal(ctx context.Context, r *http.Request) (auth.SessionView, error) {
	if h.auth == nil {
		return auth.SessionView{}, auth.ErrCredentials
	}
	if r.Header.Get("Authorization") != "" {
		raw, err := bearer(r)
		if err != nil {
			return auth.SessionView{}, err
		}
		return h.auth.TokenPrincipal(ctx, raw)
	}
	return h.auth.Me(ctx, cookieToken(r))
}
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) bool {
	return h.authorizeRead(w, r, true)
}
func (h *Handler) authorizeRead(w http.ResponseWriter, r *http.Request, allowBootstrap bool) bool {
	if r.Header.Get("Authorization") != "" && allowBootstrap && h.tokenEnabled {
		raw, err := bearer(r)
		if err == nil {
			hash := auth.HashToken(raw)
			if subtle.ConstantTimeCompare(hash, h.tokenHash[:]) == 1 {
				return true
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	view, err := h.principal(ctx, r)
	if err != nil {
		if errors.Is(err, auth.ErrCredentials) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="NetMonOrc"`)
		}
		authError(w, err)
		return false
	}
	for _, scope := range view.Scopes {
		if scope == "resources:read" {
			return true
		}
	}
	failure(w, 403, "FORBIDDEN", "resources:read required")
	return false
}
