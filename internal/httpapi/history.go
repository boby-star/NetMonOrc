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
	"strconv"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

type HistoryLoad func(context.Context, monitor.HistoryQuery) ([]monitor.ObservationRecord, error)

// SetHistoryLoader configures the handler before it is served.
func (h *Handler) SetHistoryLoader(load HistoryLoad) { h.history = load }

type ObservationView struct {
	ObservationID       string                    `json:"observationId"`
	MonitorID           string                    `json:"monitorId"`
	CheckID             string                    `json:"checkId"`
	AgentID             string                    `json:"agentId"`
	NetworkZone         string                    `json:"networkZone"`
	ConfigRevision      string                    `json:"configRevision"`
	ExecutionStatus     string                    `json:"executionStatus"`
	ProbeOutcome        string                    `json:"probeOutcome"`
	ReasonCode          string                    `json:"reasonCode,omitempty"`
	StartedAt           time.Time                 `json:"startedAt"`
	FinishedAt          time.Time                 `json:"finishedAt"`
	ReceivedAt          time.Time                 `json:"receivedAt"`
	DurationMs          float64                   `json:"durationMs"`
	Values              map[string]monitor.Scalar `json:"values,omitempty"`
	DiagnosticSessionID string                    `json:"diagnosticSessionId,omitempty"`
	DiagnosticTaskID    string                    `json:"diagnosticTaskId,omitempty"`
}
type ObservationPage struct {
	Items      []ObservationView `json:"items"`
	NextCursor *string           `json:"nextCursor"`
}
type historyCursor struct {
	Version int       `json:"v"`
	Filter  string    `json:"f"`
	At      time.Time `json:"t"`
	ID      string    `json:"id"`
}

func parseHistory(r *http.Request, name string) (monitor.HistoryQuery, string, error) {
	q := monitor.HistoryQuery{Monitor: name, Limit: 50}
	// ParseQuery errors (e.g. malformed escaping) must not silently drop a filter.
	params, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return q, "", err
	}
	for key, values := range params {
		if len(values) != 1 {
			return q, "", fmt.Errorf("duplicate query parameter")
		}
		switch key {
		case "limit", "cursor", "zone", "since", "until":
		default:
			return q, "", fmt.Errorf("unknown query parameter")
		}
	}
	if raw, ok := params["limit"]; ok {
		q.Limit, err = strconv.Atoi(raw[0])
		if err != nil || q.Limit < 1 || q.Limit > 200 {
			return q, "", fmt.Errorf("limit requires 1..200")
		}
	}
	q.Zone = params.Get("zone")
	if len(q.Zone) > 256 {
		return q, "", fmt.Errorf("zone too long")
	}
	for key, target := range map[string]*time.Time{"since": &q.Since, "until": &q.Until} {
		if raw, ok := params[key]; ok {
			*target, err = time.Parse(time.RFC3339Nano, raw[0])
			if err != nil {
				return q, "", fmt.Errorf("timestamp requires RFC3339")
			}
		}
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && q.Since.After(q.Until) {
		return q, "", fmt.Errorf("invalid time interval")
	}
	// Bind the opaque cursor to the actual monitor/filter, not arbitrary client state.
	data, _ := json.Marshal([]string{q.Monitor, q.Zone, q.Since.UTC().Format(time.RFC3339Nano), q.Until.UTC().Format(time.RFC3339Nano)})
	digest := sha256.Sum256(data)
	filter := hex.EncodeToString(digest[:])
	if raw, ok := params["cursor"]; ok {
		if len(raw[0]) == 0 || len(raw[0]) > 2048 {
			return q, "", fmt.Errorf("invalid cursor")
		}
		decoded, e := base64.RawURLEncoding.DecodeString(raw[0])
		if e != nil {
			return q, "", fmt.Errorf("invalid cursor")
		}
		var c historyCursor
		if e = json.Unmarshal(decoded, &c); e != nil || c.Version != 1 || c.Filter != filter || c.At.IsZero() || c.ID == "" || len(c.ID) > 128 {
			return q, "", fmt.Errorf("invalid cursor for filter")
		}
		q.Before, q.BeforeID = c.At, c.ID
	}
	return q, filter, nil
}
func (h *Handler) observations(w http.ResponseWriter, r *http.Request) {
	// The legacy bootstrap reader is intentionally restricted to Assessment.
	if r.Header.Get("Authorization") != "" || h.auth == nil {
		failure(w, 401, "UNAUTHORIZED", "Browser session required")
		return
	}
	if !h.authorize(w, r) {
		return
	}
	if r.Method != "GET" {
		w.Header().Set("Allow", "GET")
		failure(w, 405, "METHOD_NOT_ALLOWED", "GET required")
		return
	}
	if h.history == nil {
		failure(w, 404, "NOT_FOUND", "Observation history unavailable")
		return
	}
	if len(r.URL.RawQuery) > 4096 {
		failure(w, 400, "INVALID_ARGUMENT", "Query too long")
		return
	}
	q, filter, err := parseHistory(r, h.monitor)
	if err != nil {
		failure(w, 400, "INVALID_ARGUMENT", err.Error())
		return
	}
	select {
	case h.readSlots <- struct{}{}:
		defer func() { <-h.readSlots }()
	default:
		w.Header().Set("Retry-After", "1")
		failure(w, 429, "API_BUSY", "Monitoring reader busy")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	rows, err := h.history(ctx, q)
	if err != nil {
		failure(w, 503, "HISTORY_UNAVAILABLE", "Observation history temporarily unavailable")
		return
	}
	page := ObservationPage{Items: []ObservationView{}}
	if len(rows) > q.Limit {
		last := rows[q.Limit-1]
		b, _ := json.Marshal(historyCursor{1, filter, last.SortFinishedAt, last.Observation.ID})
		cursor := base64.RawURLEncoding.EncodeToString(b)
		page.NextCursor = &cursor
		rows = rows[:q.Limit]
	}
	for _, record := range rows {
		o := record.Observation
		page.Items = append(page.Items, ObservationView{
			ObservationID: o.ID, MonitorID: o.Monitor, CheckID: o.Check, AgentID: o.Agent, NetworkZone: o.Zone,
			ConfigRevision: o.Revision, ExecutionStatus: o.Execution, ProbeOutcome: o.Outcome, ReasonCode: o.Reason,
			StartedAt: o.StartedAt, FinishedAt: o.FinishedAt, ReceivedAt: record.ReceivedAt, DurationMs: float64(o.FinishedAt.Sub(o.StartedAt)) / float64(time.Millisecond),
			Values: o.Values, DiagnosticSessionID: record.DiagnosticSessionID, DiagnosticTaskID: o.DiagnosticTaskID,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(page)
}
