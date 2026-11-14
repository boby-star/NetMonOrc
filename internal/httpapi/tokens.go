package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
)

func (h *Handler) tokenRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if len(r.URL.RawQuery) > 4096 {
		failure(w, 400, "INVALID_ARGUMENT", "Query too long")
		return
	}
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if r.Method != "GET" {
		if !sameOrigin(r) {
			failure(w, 403, "ORIGIN_REJECTED", "Same-origin request required")
			return
		}
		if r.Header.Get("Authorization") == "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(principal.CSRFToken)) != 1 {
			failure(w, 403, "CSRF_REJECTED", "Valid CSRF token required")
			return
		}
	}
	if r.URL.Path == "/api/v1/auth/tokens" {
		switch r.Method {
		case "GET":
			params, e := url.ParseQuery(r.URL.RawQuery)
			if e != nil {
				failure(w, 400, "INVALID_ARGUMENT", "Invalid query")
				return
			}
			for key, values := range params {
				if (key != "limit" && key != "cursor") || len(values) != 1 {
					failure(w, 400, "INVALID_ARGUMENT", "Invalid query parameters")
					return
				}
			}
			limit := 50
			if values, ok := params["limit"]; ok {
				limit, e = strconv.Atoi(values[0])
				if e != nil || limit < 1 || limit > 200 {
					failure(w, 400, "INVALID_ARGUMENT", "limit requires 1..200")
					return
				}
			}
			cursor := params.Get("cursor")
			if len(cursor) > 128 {
				failure(w, 400, "INVALID_ARGUMENT", "Invalid cursor")
				return
			}
			page, e := h.auth.Tokens(ctx, principal.UserID, cursor, limit)
			if e != nil {
				authError(w, e)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(page)
		case "POST":
			if r.URL.RawQuery != "" {
				failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
				return
			}
			// A PAT may manage its own metadata but cannot mint another credential.
			if r.Header.Get("Authorization") != "" {
				failure(w, 403, "FORBIDDEN", "Browser session required to issue token")
				return
			}
			kind, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if e != nil || kind != "application/json" {
				failure(w, 400, "INVALID_ARGUMENT", "JSON required")
				return
			}
			var in auth.APITokenCreate
			decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
			decoder.DisallowUnknownFields()
			if e = decoder.Decode(&in); e != nil {
				failure(w, 400, "INVALID_ARGUMENT", "Invalid token request")
				return
			}
			if decoder.Decode(new(any)) != io.EOF {
				failure(w, 400, "INVALID_ARGUMENT", "One token request required")
				return
			}
			issued, e := h.auth.IssueToken(ctx, cookieToken(r), in)
			if e != nil {
				authError(w, e)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(issued)
		default:
			w.Header().Set("Allow", "GET, POST")
			failure(w, 405, "METHOD_NOT_ALLOWED", "GET or POST required")
		}
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/auth/tokens/")
	if id == "" || strings.Contains(id, "/") {
		failure(w, 404, "NOT_FOUND", "Token endpoint not found")
		return
	}
	if r.Method != "DELETE" {
		w.Header().Set("Allow", "DELETE")
		failure(w, 405, "METHOD_NOT_ALLOWED", "DELETE required")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	if err = h.auth.RevokeToken(ctx, principal.UserID, id); err != nil {
		authError(w, err)
		return
	}
	w.WriteHeader(204)
}
