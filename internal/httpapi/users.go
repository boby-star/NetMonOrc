package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
)

func userETag(v auth.UserView) string { return fmt.Sprintf(`"user-%s-%d"`, v.ID, v.Version) }
func userJSON(w http.ResponseWriter, r *http.Request, in any) bool {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		failure(w, 400, "INVALID_ARGUMENT", "JSON required")
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(in); err != nil {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid user request")
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		failure(w, 400, "INVALID_ARGUMENT", "One request required")
		return false
	}
	return true
}
func writeUser(w http.ResponseWriter, status int, v auth.UserView) {
	w.Header().Set("ETag", userETag(v))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) userRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if !auth.IsAdmin(principal) {
		authError(w, auth.ErrForbidden)
		return
	}
	actor := auth.AdminCredential{UserID: principal.UserID}
	if r.Header.Get("Authorization") != "" {
		raw, e := bearer(r)
		if e != nil {
			authError(w, e)
			return
		}
		actor.Hash = auth.HashToken(raw)
		actor.PAT = true
	} else {
		actor.Hash = auth.HashToken(cookieToken(r))
	}
	if r.Method != "GET" {
		if !sameOrigin(r) {
			failure(w, 403, "ORIGIN_REJECTED", "Same-origin request required")
			return
		}
		if !actor.PAT && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(principal.CSRFToken)) != 1 {
			failure(w, 403, "CSRF_REJECTED", "Valid CSRF token required")
			return
		}
	}
	if len(r.URL.RawQuery) > 4096 {
		failure(w, 400, "INVALID_ARGUMENT", "Query too long")
		return
	}
	if r.URL.Path == "/api/v1/users" {
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
			if raw, ok := params["limit"]; ok {
				limit, e = strconv.Atoi(raw[0])
				if e != nil || limit < 1 || limit > 200 {
					failure(w, 400, "INVALID_ARGUMENT", "limit requires 1..200")
					return
				}
			}
			page, e := h.auth.Users(ctx, principal, params.Get("cursor"), limit)
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
			var input auth.UserCreate
			if !userJSON(w, r, &input) {
				return
			}
			view, e := h.auth.CreateUser(ctx, actor, input)
			input.InitialPassword = ""
			if e != nil {
				authError(w, e)
				return
			}
			w.Header().Set("Location", "/api/v1/users/"+view.ID)
			writeUser(w, 201, view)
		default:
			w.Header().Set("Allow", "GET, POST")
			failure(w, 405, "METHOD_NOT_ALLOWED", "GET or POST required")
		}
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/users/")
	if id == "" || strings.Contains(id, "/") {
		failure(w, 404, "NOT_FOUND", "User endpoint not found")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	switch r.Method {
	case "GET":
		view, e := h.auth.User(ctx, principal, id)
		if e != nil {
			authError(w, e)
			return
		}
		writeUser(w, 200, view)
	case "PATCH":
		tag := r.Header.Get("If-Match")
		if tag == "" {
			failure(w, 428, "PRECONDITION_REQUIRED", "If-Match required")
			return
		}
		if len(tag) < 2 || len(tag) > 256 || !strings.HasPrefix(tag, `"`) || !strings.HasSuffix(tag, `"`) || strings.ContainsAny(tag[1:len(tag)-1], "\"\r\n") {
			failure(w, 400, "INVALID_ARGUMENT", "Single strong ETag required")
			return
		}
		prefix := `"user-` + id + "-"
		if !strings.HasPrefix(tag, prefix) {
			authError(w, auth.ErrPrecondition)
			return
		}
		expected, e := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(tag, prefix), `"`), 10, 64)
		if e != nil || expected < 1 {
			authError(w, auth.ErrPrecondition)
			return
		}
		var wire struct {
			Role     json.RawMessage `json:"role"`
			Disabled json.RawMessage `json:"disabled"`
		}
		if !userJSON(w, r, &wire) {
			return
		}
		patch := auth.UserPatch{}
		if len(wire.Role) > 0 {
			var role string
			if json.Unmarshal(wire.Role, &role) != nil || bytes.Equal(bytes.TrimSpace(wire.Role), []byte("null")) {
				authError(w, auth.ErrInput)
				return
			}
			patch.Role = &role
		}
		if len(wire.Disabled) > 0 {
			var disabled bool
			if json.Unmarshal(wire.Disabled, &disabled) != nil || bytes.Equal(bytes.TrimSpace(wire.Disabled), []byte("null")) {
				authError(w, auth.ErrInput)
				return
			}
			patch.Disabled = &disabled
		}
		view, e := h.auth.PatchUser(ctx, actor, id, expected, patch)
		if e != nil {
			authError(w, e)
			return
		}
		writeUser(w, 200, view)
	default:
		w.Header().Set("Allow", "GET, PATCH")
		failure(w, 405, "METHOD_NOT_ALLOWED", "GET or PATCH required")
	}
}
