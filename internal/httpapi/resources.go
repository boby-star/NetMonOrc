package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func (h *Handler) SetResources(service *resources.Service, revision string) {
	h.resources = service
	h.expectedRevision = revision
}
func resourceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, resources.ErrPreconditionRequired):
		failure(w, 428, "PRECONDITION_REQUIRED", "Existing resources require expectedVersions")
	case errors.Is(err, resources.ErrIdempotency):
		failure(w, 409, "IDEMPOTENCY_KEY_REUSED", "Request ID was already used for another body")
	case errors.Is(err, resources.ErrNotFound):
		failure(w, 404, "NOT_FOUND", "Resource unavailable")
	case errors.Is(err, resources.ErrVersion):
		failure(w, 412, "PRECONDITION_FAILED", "Resource version changed; reload first")
	case errors.Is(err, resources.ErrConflict):
		failure(w, 409, "CONFLICT", "Resource exists or is selected by Controller")
	case errors.Is(err, resources.ErrInvalid):
		failure(w, 422, "INVALID_RESOURCE", "Invalid or unsupported resource")
	default:
		authError(w, err)
	}
}
func resourceJSON(w http.ResponseWriter, r *http.Request, in any) bool {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		failure(w, 400, "INVALID_ARGUMENT", "JSON required")
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err = d.Decode(in); err != nil {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid resource request")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		failure(w, 400, "INVALID_ARGUMENT", "One request required")
		return false
	}
	return true
}
func resourceMatch(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := r.Header.Get("If-Match")
	if raw == "" {
		failure(w, 428, "PRECONDITION_REQUIRED", "If-Match required")
		return "", false
	}
	if len(r.Header.Values("If-Match")) != 1 || len(raw) < 3 || len(raw) > 200 || raw[0] != '"' || raw[len(raw)-1] != '"' || strings.ContainsAny(raw[1:len(raw)-1], "\"\r\n") {
		failure(w, 400, "INVALID_ARGUMENT", "Single strong ETag required")
		return "", false
	}
	return raw[1 : len(raw)-1], true
}
func resourceView(w http.ResponseWriter, status int, v resources.View) {
	w.Header().Set("ETag", `"`+v.ResourceVersion+`"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) resourceRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	configuration := strings.HasPrefix(r.URL.Path, "/api/v1/config/")
	write := r.Method != "GET" || configuration
	scope := "resources:read"
	if write {
		scope = "resources:write"
	}
	if !slices.Contains(principal.Scopes, scope) || (write && principal.Role != "admin") {
		authError(w, auth.ErrForbidden)
		return
	}
	if write {
		if !sameOrigin(r) {
			failure(w, 403, "ORIGIN_REJECTED", "Same-origin required")
			return
		}
		if r.Header.Get("Authorization") == "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(principal.CSRFToken)) != 1 {
			failure(w, 403, "CSRF_REJECTED", "Valid CSRF token required")
			return
		}
	}
	if h.resources == nil {
		failure(w, 404, "NOT_FOUND", "Resource management unavailable")
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
	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid query")
		return
	}
	limit := 50
	for key, values := range params {
		if !slices.Contains([]string{"limit", "cursor", "labelSelector"}, key) || len(values) != 1 {
			failure(w, 400, "INVALID_ARGUMENT", "Unsupported or duplicate query parameter")
			return
		}
	}
	if raw, ok := params["limit"]; ok {
		limit, err = strconv.Atoi(raw[0])
		if err != nil || limit < 1 || limit > 200 {
			failure(w, 400, "INVALID_ARGUMENT", "limit requires 1..200")
			return
		}
	}
	if configuration {
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			failure(w, 405, "METHOD_NOT_ALLOWED", "POST required")
			return
		}
		if r.URL.RawQuery != "" {
			failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
			return
		}
		if r.URL.Path != "/api/v1/config/validate" && r.URL.Path != "/api/v1/config/plan" && r.URL.Path != "/api/v1/config/apply" {
			failure(w, 404, "NOT_FOUND", "Configuration operation unavailable")
			return
		}
		var batch resources.Batch
		if !resourceJSON(w, r, &batch) {
			return
		}
		if r.URL.Path == "/api/v1/config/apply" {
			actor := auth.AdminCredential{UserID: principal.UserID, Hash: auth.HashToken(cookieToken(r))}
			if r.Header.Get("Authorization") != "" {
				raw, _ := bearer(r)
				actor.Hash = auth.HashToken(raw)
				actor.PAT = true
			}
			op, e := h.resources.Apply(ctx, actor, batch)
			if e != nil {
				resourceError(w, e)
				return
			}
			w.Header().Set("Location", "/api/v1/operations/"+op.ID)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			_ = json.NewEncoder(w).Encode(op)
			return
		}
		validation := h.resources.Validate(batch)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/config/validate" {
			_ = json.NewEncoder(w).Encode(validation)
			return
		}
		if !validation.Valid {
			failure(w, 422, "INVALID_RESOURCE", "Batch failed validation")
			return
		}
		plan, e := h.resources.Plan(ctx, batch)
		if e != nil {
			resourceError(w, e)
			return
		}
		_ = json.NewEncoder(w).Encode(plan)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/resources/"), "/")
	if len(parts) < 1 || parts[0] != "Monitor" {
		failure(w, 422, "UNSUPPORTED_KIND", "This slice manages Monitor resources only")
		return
	}
	if r.Method == "GET" && len(parts) == 1 {
		rows, e := h.resources.Repo.Resources(ctx, params.Get("cursor"), params.Get("labelSelector"), limit)
		if e != nil {
			resourceError(w, e)
			return
		}
		page := resources.Page{Items: []resources.View{}}
		if len(rows) > limit {
			cursor := rows[limit-1].Manifest.Metadata.Name
			page.NextCursor = &cursor
			rows = rows[:limit]
		}
		for _, v := range rows {
			page.Items = append(page.Items, h.resources.Status(v, h.expectedRevision))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
		return
	}
	name := ""
	if len(parts) >= 2 {
		name = parts[1]
	}
	if len(parts) == 3 && r.Method == "GET" {
		switch parts[2] {
		case "status":
			if r.URL.RawQuery != "" {
				failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
				return
			}
			v, found, e := h.resources.Repo.Resource(ctx, name)
			if e != nil {
				resourceError(w, e)
				return
			}
			if !found {
				resourceError(w, resources.ErrNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(h.resources.Status(v, h.expectedRevision).Status)
			return
		case "revisions":
			if params.Get("labelSelector") != "" {
				failure(w, 400, "INVALID_ARGUMENT", "No labelSelector for revisions")
				return
			}
			rows, e := h.resources.Repo.ResourceHistory(ctx, name, params.Get("cursor"), limit)
			if e != nil {
				resourceError(w, e)
				return
			}
			var cursor *string
			if len(rows) > limit {
				last := rows[limit-1].ResourceVersion
				cursor = &last
				rows = rows[:limit]
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(struct {
				Items      []resources.Revision `json:"items"`
				NextCursor *string              `json:"nextCursor"`
			}{rows, cursor})
			return
		}
	}
	if len(parts) > 2 {
		failure(w, 404, "NOT_FOUND", "Resource endpoint unavailable")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	if len(parts) == 2 && r.Method == "GET" {
		v, found, e := h.resources.Repo.Resource(ctx, name)
		if e != nil {
			resourceError(w, e)
			return
		}
		if !found {
			resourceError(w, resources.ErrNotFound)
			return
		}
		resourceView(w, 200, h.resources.Status(v, h.expectedRevision))
		return
	}
	actor := auth.AdminCredential{UserID: principal.UserID, Hash: auth.HashToken(cookieToken(r))}
	if r.Header.Get("Authorization") != "" {
		raw, _ := bearer(r)
		actor.Hash = auth.HashToken(raw)
		actor.PAT = true
	}
	if (len(parts) == 1 && r.Method == "POST") || (len(parts) == 2 && r.Method == "PUT") {
		var expected *string
		if r.Method == "PUT" {
			version, ok := resourceMatch(w, r)
			if !ok {
				return
			}
			expected = &version
		}
		var m resources.Manifest
		if !resourceJSON(w, r, &m) {
			return
		}
		if m.Kind != "Monitor" || (name != "" && m.Metadata.Name != name) {
			failure(w, 422, "IDENTITY_MISMATCH", "Manifest kind/name must match route")
			return
		}
		compiled, e := h.resources.Registry.Compile(m)
		if e != nil {
			failure(w, 422, "INVALID_MANIFEST", e.Error())
			return
		}
		v, e := h.resources.Repo.SaveResource(ctx, actor, m, compiled.Revision, expected)
		if e != nil {
			resourceError(w, e)
			return
		}
		status := 200
		if r.Method == "POST" {
			status = 201
			w.Header().Set("Location", "/api/v1/resources/Monitor/"+m.Metadata.Name)
		}
		resourceView(w, status, h.resources.Status(v, h.expectedRevision))
		return
	}
	if len(parts) == 2 && r.Method == "DELETE" {
		version, ok := resourceMatch(w, r)
		if !ok {
			return
		}
		if name == h.resources.Registry.Active {
			resourceError(w, resources.ErrConflict)
			return
		}
		if e := h.resources.Repo.DeleteResource(ctx, actor, name, version); e != nil {
			resourceError(w, e)
			return
		}
		w.WriteHeader(204)
		return
	}
	w.Header().Set("Allow", "GET, POST, PUT, DELETE")
	failure(w, 405, "METHOD_NOT_ALLOWED", "Unsupported resource method")
}
