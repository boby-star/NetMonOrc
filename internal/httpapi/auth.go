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
	for _, c := range r.Cookies() {
		if c.Name == cookieName {
			if raw != "" {
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
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: raw, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, Expires: view.ExpiresAt, MaxAge: int(auth.SessionTTL.Seconds())})
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	case "/api/v1/auth/me":
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			failure(w, 405, "METHOD_NOT_ALLOWED", "GET required")
			return
		}
		view, err := h.auth.Me(ctx, cookieToken(r))
		if err != nil {
			authError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(view)
	case "/api/v1/auth/logout":
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
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) bool {
	if header := r.Header.Get("Authorization"); header != "" {
		scheme, value, present := strings.Cut(header, " ")
		hash := auth.HashToken(value)
		if h.tokenEnabled && present && strings.EqualFold(scheme, "Bearer") && subtle.ConstantTimeCompare(hash, h.tokenHash[:]) == 1 {
			return true
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="NetMonOrc"`)
		authError(w, auth.ErrCredentials)
		return false
	}
	if h.auth != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		view, err := h.auth.Me(ctx, cookieToken(r))
		if err != nil {
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
	w.Header().Set("WWW-Authenticate", `Bearer realm="NetMonOrc"`)
	authError(w, auth.ErrCredentials)
	return false
}
