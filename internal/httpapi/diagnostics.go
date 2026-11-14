package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
)

func (h *Handler) SetDiagnostics(reader diagnosticview.Reader) { h.diagnostics = reader }
func diagnosticQuery(r *http.Request, tasks bool) (diagnosticview.Query, string, error) {
	q := diagnosticview.Query{Limit: 50}
	if len(r.URL.RawQuery) > 4096 {
		return q, "", fmt.Errorf("query too long")
	}
	p, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return q, "", err
	}
	allowed := []string{"limit", "cursor"}
	if !tasks {
		allowed = append(allowed, "monitor", "state", "since", "until")
	}
	for k, v := range p {
		if len(v) != 1 || !slices.Contains(allowed, k) {
			return q, "", fmt.Errorf("unsupported or duplicate filter")
		}
	}
	if raw, ok := p["limit"]; ok {
		q.Limit, err = strconv.Atoi(raw[0])
		if err != nil || q.Limit < 1 || q.Limit > 200 {
			return q, "", fmt.Errorf("limit requires 1..200")
		}
	}
	q.Monitor = p.Get("monitor")
	q.State = p.Get("state")
	if len(q.Monitor) > 63 || (q.State != "" && !slices.Contains([]string{"created", "running", "completed", "inconclusive", "cancelled"}, q.State)) {
		return q, "", fmt.Errorf("invalid monitor or state")
	}
	for k, dest := range map[string]*time.Time{"since": &q.Since, "until": &q.Until} {
		if raw, ok := p[k]; ok {
			*dest, err = time.Parse(time.RFC3339Nano, raw[0])
			if err != nil {
				return q, "", fmt.Errorf("RFC3339 timestamp required")
			}
		}
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && q.Since.After(q.Until) {
		return q, "", fmt.Errorf("invalid time interval")
	}
	identity, _ := json.Marshal([]string{r.URL.Path, q.Monitor, q.State, q.Since.UTC().Format(time.RFC3339Nano), q.Until.UTC().Format(time.RFC3339Nano)})
	sum := sha256.Sum256(identity)
	filter := hex.EncodeToString(sum[:])
	if raw, ok := p["cursor"]; ok {
		if len(raw[0]) == 0 || len(raw[0]) > 2048 {
			return q, "", fmt.Errorf("invalid cursor")
		}
		data, e := base64.RawURLEncoding.DecodeString(raw[0])
		if e != nil {
			return q, "", fmt.Errorf("invalid cursor")
		}
		var c historyCursor
		if e = json.Unmarshal(data, &c); e != nil || c.Version != 1 || c.Filter != filter || c.ID == "" || len(c.ID) > 128 || (!tasks && c.At.IsZero()) {
			return q, "", fmt.Errorf("cursor does not match filters")
		}
		q.BeforeID = c.ID
		q.Before = c.At
	}
	return q, filter, nil
}
func diagnosticCursor(filter, id string, at time.Time) *string {
	data, _ := json.Marshal(historyCursor{Version: 1, Filter: filter, At: at, ID: id})
	value := base64.RawURLEncoding.EncodeToString(data)
	return &value
}
func (h *Handler) diagnosticRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if !slices.Contains(principal.Scopes, "diagnostics:read") {
		authError(w, auth.ErrForbidden)
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "Diagnostic history is read-only")
		return
	}
	if h.diagnostics == nil {
		failure(w, 404, "NOT_FOUND", "Diagnostic history unavailable")
		return
	}
	select {
	case h.readSlots <- struct{}{}:
		defer func() { <-h.readSlots }()
	default:
		authError(w, auth.ErrBusy)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/v1/diagnostics" {
		q, filter, e := diagnosticQuery(r, false)
		if e != nil {
			failure(w, 400, "INVALID_ARGUMENT", e.Error())
			return
		}
		rows, e := h.diagnostics.DiagnosticSessions(ctx, q)
		if e != nil {
			failure(w, 503, "DIAGNOSTICS_UNAVAILABLE", "Diagnostic history temporarily unavailable")
			return
		}
		page := diagnosticview.Sessions{Items: rows}
		if len(rows) > q.Limit {
			last := rows[q.Limit-1]
			page.NextCursor = diagnosticCursor(filter, last.ID, last.StartedAt)
			page.Items = rows[:q.Limit]
		}
		_ = json.NewEncoder(w).Encode(page)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/diagnostics/"), "/")
	if len(parts) > 2 || parts[0] == "" || len(parts[0]) > 128 || (len(parts) == 2 && parts[1] != "tasks") {
		failure(w, 404, "NOT_FOUND", "Diagnostic session unavailable")
		return
	}
	if len(parts) == 1 && r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	var q diagnosticview.Query
	var filter string
	if len(parts) == 2 {
		q, filter, err = diagnosticQuery(r, true)
		if err != nil {
			failure(w, 400, "INVALID_ARGUMENT", err.Error())
			return
		}
	}
	v, found, err := h.diagnostics.DiagnosticSession(ctx, parts[0])
	if err != nil {
		failure(w, 503, "DIAGNOSTICS_UNAVAILABLE", "Diagnostic session temporarily unavailable")
		return
	}
	if !found {
		failure(w, 404, "NOT_FOUND", "Diagnostic session unavailable")
		return
	}
	if len(parts) == 1 {
		_ = json.NewEncoder(w).Encode(v)
		return
	}
	rows, err := h.diagnostics.DiagnosticTasks(ctx, parts[0], q.BeforeID, q.Limit)
	if err != nil {
		failure(w, 503, "DIAGNOSTICS_UNAVAILABLE", "Diagnostic tasks temporarily unavailable")
		return
	}
	page := diagnosticview.Tasks{Items: rows}
	if len(rows) > q.Limit {
		last := rows[q.Limit-1]
		page.NextCursor = diagnosticCursor(filter, last.ID, time.Time{})
		page.Items = rows[:q.Limit]
	}
	_ = json.NewEncoder(w).Encode(page)
}
