package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
	"github.com/jackc/pgx/v5"
)

// No target, check_spec, response body or extracted values are exposed here.
const diagnosticViewColumns = `r.id,r.monitor,r.profile_ref,r.mode,r.state,r.started_at,r.finished_at,r.used_checks,r.reason,r.report->'hypotheses',
 ARRAY(SELECT o.id FROM nmo_diagnostic_tasks t JOIN nmo_observations o ON o.diagnostic_task_id=t.id WHERE t.run_id=r.id AND o.diagnostic_task_id<>'' ORDER BY o.id)`

func scanDiagnosticView(row pgx.Row) (diagnosticview.Session, error) {
	var v diagnosticview.Session
	var reason string
	var report []byte
	err := row.Scan(&v.ID, &v.MonitorID, &v.ProfileRef, &v.Mode, &v.State, &v.StartedAt, &v.FinishedAt, &v.BudgetUsed.AdditionalChecks, &reason, &report, &v.EvidenceIDs)
	if err != nil {
		return v, err
	}
	if reason != "" {
		v.StopReason = &reason
	}
	// Missing report after a restart is unknown, not an empty hypothesis set.
	if len(report) > 0 && string(report) != "null" {
		var hypotheses []string
		if err = json.Unmarshal(report, &hypotheses); err != nil {
			return v, err
		}
		v.RemainingHypotheses = &hypotheses
	}
	return v, nil
}
func (s *Store) DiagnosticSessions(ctx context.Context, q diagnosticview.Query) ([]diagnosticview.Session, error) {
	if q.Limit < 1 || q.Limit > 200 {
		return nil, fmt.Errorf("invalid bounded diagnostic query")
	}
	rows, err := s.pool.Query(ctx, `SELECT `+diagnosticViewColumns+` FROM nmo_diagnostic_runs r
 WHERE ($1='' OR r.monitor=$1) AND ($2='' OR r.state=$2)
 AND ($3::timestamptz IS NULL OR r.started_at >= $3) AND ($4::timestamptz IS NULL OR r.started_at <= $4)
 AND ($5::timestamptz IS NULL OR (r.started_at,r.id)<($5,$6)) ORDER BY r.started_at DESC,r.id DESC LIMIT $7`, q.Monitor, q.State, historyTime(q.Since), historyTime(q.Until), historyTime(q.Before), q.BeforeID, q.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []diagnosticview.Session{}
	for rows.Next() {
		v, e := scanDiagnosticView(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) DiagnosticSession(ctx context.Context, id string) (diagnosticview.Session, bool, error) {
	v, err := scanDiagnosticView(s.pool.QueryRow(ctx, `SELECT `+diagnosticViewColumns+` FROM nmo_diagnostic_runs r WHERE r.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, err
}
func (s *Store) DiagnosticTasks(ctx context.Context, id, before string, limit int) ([]diagnosticview.Task, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("invalid bounded task query")
	}
	rows, err := s.pool.Query(ctx, `SELECT t.id,t.run_id,t.check_id,t.agent_id,t.state,t.attempt_no,t.deadline,
 ARRAY(SELECT o.id FROM nmo_observations o WHERE o.diagnostic_task_id=t.id AND o.diagnostic_task_id<>'' ORDER BY o.id)
 FROM nmo_diagnostic_tasks t WHERE t.run_id=$1 AND t.id>$2 ORDER BY t.id ASC LIMIT $3`, id, before, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []diagnosticview.Task{}
	for rows.Next() {
		var v diagnosticview.Task
		if err = rows.Scan(&v.ID, &v.SessionID, &v.ActionID, &v.AgentID, &v.State, &v.AttemptNo, &v.Deadline, &v.ObservationIDs); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
