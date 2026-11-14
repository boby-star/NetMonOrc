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

	"github.com/boby-star/NetMonOrc/internal/alertview"
	"github.com/boby-star/NetMonOrc/internal/auth"
)

func (h *Handler) SetAlerts(reader alertview.Reader, endpointHash string) {
	h.alerts = reader
	h.alertEndpoint = endpointHash
}
func alertQuery(r *http.Request, kind, endpoint string) (alertview.Query, string, error) {
	q := alertview.Query{Limit: 50, EndpointHash: endpoint}
	if len(r.URL.RawQuery) > 4096 {
		return q, "", fmt.Errorf("query too long")
	}
	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return q, "", err
	}
	allowed := []string{"limit", "cursor"}
	if kind == "alerts" {
		allowed = append(allowed, "state", "severity", "monitor", "zone")
	}
	if kind == "deliveries" {
		allowed = append(allowed, "state")
	}
	for key, values := range params {
		if len(values) != 1 || !slices.Contains(allowed, key) {
			return q, "", fmt.Errorf("unsupported or duplicate filter")
		}
	}
	if raw, ok := params["limit"]; ok {
		q.Limit, err = strconv.Atoi(raw[0])
		if err != nil || q.Limit < 1 || q.Limit > 200 {
			return q, "", fmt.Errorf("limit requires 1..200")
		}
	}
	q.State = params.Get("state")
	q.Severity = params.Get("severity")
	q.Monitor = params.Get("monitor")
	q.Zone = params.Get("zone")
	states := []string{"normal", "pending", "firing", "resolved"}
	if kind == "deliveries" {
		states = []string{"pending", "leased", "delivered", "failed", "cancelled"}
	}
	if q.State != "" && !slices.Contains(states, q.State) || q.Severity != "" && !slices.Contains([]string{"info", "warning", "critical"}, q.Severity) || len(q.Monitor) > 63 || len(q.Zone) > 63 {
		return q, "", fmt.Errorf("invalid filter")
	}
	identity, _ := json.Marshal([]string{r.URL.Path, q.State, q.Severity, q.Monitor, q.Zone, endpoint})
	sum := sha256.Sum256(identity)
	filter := hex.EncodeToString(sum[:])
	if raw, ok := params["cursor"]; ok {
		if len(raw[0]) == 0 || len(raw[0]) > 2048 {
			return q, "", fmt.Errorf("invalid cursor")
		}
		data, e := base64.RawURLEncoding.DecodeString(raw[0])
		if e != nil {
			return q, "", fmt.Errorf("invalid cursor")
		}
		var c historyCursor
		if e = json.Unmarshal(data, &c); e != nil || c.Version != 1 || c.Filter != filter || c.ID == "" || len(c.ID) > 128 || (kind == "history" && c.At.IsZero()) {
			return q, "", fmt.Errorf("cursor does not match filters")
		}
		q.Before = c.ID
		q.BeforeAt = c.At
	}
	return q, filter, nil
}
func (h *Handler) alertRoutes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	principal, err := h.principal(ctx, r)
	if err != nil {
		authError(w, err)
		return
	}
	if !slices.Contains(principal.Scopes, "alerts:read") {
		authError(w, auth.ErrForbidden)
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "Alert history is read-only")
		return
	}
	if h.alerts == nil {
		failure(w, 404, "NOT_FOUND", "Alert history unavailable")
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
	if r.URL.Path == "/api/v1/alerts" || r.URL.Path == "/api/v1/webhook/deliveries" {
		kind := "alerts"
		endpoint := ""
		if r.URL.Path == "/api/v1/webhook/deliveries" {
			kind = "deliveries"
			endpoint = h.alertEndpoint
		}
		q, filter, e := alertQuery(r, kind, endpoint)
		if e != nil {
			failure(w, 400, "INVALID_ARGUMENT", e.Error())
			return
		}
		if kind == "alerts" {
			rows, e := h.alerts.Alerts(ctx, q)
			if e != nil {
				alertReadError(w)
				return
			}
			page := alertview.AlertPage{Items: rows}
			if len(rows) > q.Limit {
				page.Items = rows[:q.Limit]
				page.NextCursor = diagnosticCursor(filter, page.Items[q.Limit-1].ID, time.Time{})
			}
			_ = json.NewEncoder(w).Encode(page)
			return
		}
		rows, e := h.alerts.WebhookDeliveries(ctx, q)
		if e != nil {
			alertReadError(w)
			return
		}
		page := alertview.DeliveryPage{Items: rows}
		if len(rows) > q.Limit {
			page.Items = rows[:q.Limit]
			page.NextCursor = diagnosticCursor(filter, page.Items[q.Limit-1].ID, time.Time{})
		}
		_ = json.NewEncoder(w).Encode(page)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/alerts/"), "/")
	if len(parts) > 2 || parts[0] == "" || len(parts[0]) > 128 || (len(parts) == 2 && parts[1] != "history") {
		failure(w, 404, "NOT_FOUND", "Alert unavailable")
		return
	}
	if len(parts) == 1 && r.URL.RawQuery != "" {
		failure(w, 400, "INVALID_ARGUMENT", "No query parameters supported")
		return
	}
	var q alertview.Query
	var filter string
	if len(parts) == 2 {
		q, filter, err = alertQuery(r, "history", "")
		if err != nil {
			failure(w, 400, "INVALID_ARGUMENT", err.Error())
			return
		}
	}
	v, found, err := h.alerts.Alert(ctx, parts[0])
	if err != nil {
		alertReadError(w)
		return
	}
	if !found {
		failure(w, 404, "NOT_FOUND", "Alert unavailable")
		return
	}
	if len(parts) == 1 {
		_ = json.NewEncoder(w).Encode(v)
		return
	}
	rows, err := h.alerts.AlertHistory(ctx, parts[0], q)
	if err != nil {
		alertReadError(w)
		return
	}
	page := alertview.HistoryPage{Items: rows}
	if len(rows) > q.Limit {
		page.Items = rows[:q.Limit]
		last := page.Items[q.Limit-1]
		page.NextCursor = diagnosticCursor(filter, last.ID, last.SortAt)
	}
	_ = json.NewEncoder(w).Encode(page)
}
func alertReadError(w http.ResponseWriter) {
	failure(w, 503, "ALERT_HISTORY_UNAVAILABLE", "Alert history temporarily unavailable")
}
