package httpapi

import (
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
)

var resourceNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type GroupAssessmentLoad func(context.Context, string) (resourcegroup.Assessment, bool, error)

func (h *Handler) SetResourceGroups(load GroupAssessmentLoad) { h.groups = load }
func (h *Handler) groupAssessment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if !slices.Contains(principal.Scopes, "resources:read") {
		authError(w, auth.ErrForbidden)
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "GET required")
		return
	}
	name := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/resource-groups/"), "/assessment")
	if !resourceNamePattern.MatchString(name) || r.URL.Path != "/api/v1/resource-groups/"+name+"/assessment" {
		failure(w, 404, "NOT_FOUND", "Group unavailable")
		return
	}
	if r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	if h.groups == nil {
		failure(w, 404, "NOT_FOUND", "Group unavailable")
		return
	}
	select {
	case h.readSlots <- struct{}{}:
		defer func() { <-h.readSlots }()
	default:
		authError(w, auth.ErrBusy)
		return
	}
	result, found, err := h.groups(ctx, name)
	if err != nil {
		failure(w, 503, "ASSESSMENT_UNAVAILABLE", "Group assessment temporarily unavailable")
		return
	}
	if !found {
		failure(w, 404, "NOT_FOUND", "Group unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(result)
}
