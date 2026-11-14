package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/alertview"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/jackc/pgx/v5"
)

// A firing projection retains its triggering evidence until a real transition.
// It never reconstructs a legacy firing cause from the latest assessment.
func projectAlert(previous []byte, id string, old alerting.State, next alerting.Evaluation, rule alerting.Rule, a monitor.Assessment) (alertview.Alert, error) {
	v := alertview.Alert{ID: id, MonitorID: a.Monitor, RuleRef: rule.Name, State: next.State.Status, Severity: rule.Severity, ReasonCodes: []string{}, EvidenceIDs: []string{}}
	if len(previous) > 0 {
		if err := json.Unmarshal(previous, &v); err != nil {
			return v, err
		}
	}
	if len(previous) > 0 && !a.At.After(old.LastEvaluatedAt) {
		return v, nil
	}
	v.ID = id
	v.MonitorID = a.Monitor
	v.RuleRef = rule.Name
	v.Severity = rule.Severity
	v.State = next.State.Status
	if old.Status == "" {
		at := a.At
		v.StartedAt = &at
	}
	freshCycle := (old.Status == "normal" || old.Status == "resolved") && (v.State == "pending" || v.State == "firing")
	if freshCycle {
		at := a.At
		v.StartedAt = &at
		v.FiredAt = nil
		v.ResolvedAt = nil
		v.AcknowledgedAt = nil
	}
	if v.State == "pending" {
		v.StartedAt = next.State.PendingSince
	}
	if next.Transition != nil {
		at := next.Transition.At
		if v.State == "firing" {
			v.FiredAt = &at
		}
		if v.State == "resolved" {
			v.ResolvedAt = &at
		}
	}
	if a.At.After(old.LastEvaluatedAt) && (old.Status != "firing" || v.State != "firing") {
		reasons, evidence := map[string]bool{}, map[string]bool{}
		for _, zone := range a.Zones {
			if zone.Reason != "" {
				reasons[zone.Reason] = true
			}
			for _, id := range zone.Evidence {
				evidence[id] = true
			}
		}
		v.ReasonCodes = []string{}
		v.EvidenceIDs = []string{}
		for reason := range reasons {
			v.ReasonCodes = append(v.ReasonCodes, reason)
		}
		for id := range evidence {
			v.EvidenceIDs = append(v.EvidenceIDs, id)
		}
		sort.Strings(v.ReasonCodes)
		sort.Strings(v.EvidenceIDs)
	}
	return v, nil
}
func (s *Store) Alerts(ctx context.Context, q alertview.Query) ([]alertview.Alert, error) {
	if q.Limit < 1 || q.Limit > 200 {
		return nil, fmt.Errorf("invalid bounded alert query")
	}
	rows, err := s.pool.Query(ctx, `SELECT api_view FROM nmo_alerts WHERE api_view IS NOT NULL AND id>$1
 AND ($2='' OR api_view->>'state'=$2) AND ($3='' OR api_view->>'severity'=$3)
 AND ($4='' OR api_view->>'monitorId'=$4) AND ($5='' OR api_view->>'zone'=$5) ORDER BY id ASC LIMIT $6`, q.Before, q.State, q.Severity, q.Monitor, q.Zone, q.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []alertview.Alert{}
	for rows.Next() {
		var raw []byte
		var v alertview.Alert
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Alert(ctx context.Context, id string) (alertview.Alert, bool, error) {
	var raw []byte
	var v alertview.Alert
	err := s.pool.QueryRow(ctx, `SELECT api_view FROM nmo_alerts WHERE id=$1 AND api_view IS NOT NULL`, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, false, nil
	}
	if err != nil {
		return v, false, err
	}
	err = json.Unmarshal(raw, &v)
	return v, err == nil, err
}
func (s *Store) AlertHistory(ctx context.Context, id string, q alertview.Query) ([]alertview.History, error) {
	if q.Limit < 1 || q.Limit > 200 {
		return nil, fmt.Errorf("invalid bounded alert history query")
	}
	rows, err := s.pool.Query(ctx, `SELECT e.id,e.transition,e.suppressed,e.created_at FROM nmo_alert_events e JOIN nmo_alerts a ON a.logical_key=e.logical_key WHERE a.id=$1 AND ($2::timestamptz IS NULL OR (e.created_at,e.id)<($2,$3)) ORDER BY e.created_at DESC,e.id DESC LIMIT $4`, id, historyTime(q.BeforeAt), q.Before, q.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []alertview.History{}
	for rows.Next() {
		var v alertview.History
		var raw []byte
		var transition alerting.Transition
		if err = rows.Scan(&v.ID, &raw, &v.Suppressed, &v.SortAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &transition); err != nil {
			return nil, err
		}
		v.From = transition.From
		v.To = transition.To
		v.At = transition.At
		v.ConfigRevision = transition.Revision
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) WebhookDeliveries(ctx context.Context, q alertview.Query) ([]alertview.Delivery, error) {
	if q.Limit < 1 || q.Limit > 200 {
		return nil, fmt.Errorf("invalid bounded delivery query")
	}
	rows, err := s.pool.Query(ctx, `SELECT o.event_id,o.state,o.attempts,NULLIF(o.status_code,0),
 (SELECT max(a.started_at) FROM nmo_webhook_attempts a WHERE a.event_id=o.event_id)
 FROM nmo_webhook_outbox o WHERE o.endpoint_hash=$1 AND o.event_id>$2 AND ($3='' OR o.state=$3) ORDER BY o.event_id ASC LIMIT $4`, q.EndpointHash, q.Before, q.State, q.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []alertview.Delivery{}
	for rows.Next() {
		var v alertview.Delivery
		if err = rows.Scan(&v.ID, &v.State, &v.Attempts, &v.LastHTTPStatus, &v.LastAttemptAt); err != nil {
			return nil, err
		}
		v.EventID = v.ID
		out = append(out, v)
	}
	return out, rows.Err()
}
