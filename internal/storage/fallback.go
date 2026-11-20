package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/fallback"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/jackc/pgx/v5"
)

type FallbackRun = fallback.Run
type FallbackTask = fallback.Task

func (s *Store) OpenFallback(ctx context.Context, key string, r FallbackRun) (FallbackRun, error) {
	r.ID = newID()
	r.State = "pending"
	r.Tasks = []FallbackTask{}
	b, err := json.Marshal(r)
	if err != nil {
		return r, err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO nmo_fallback_runs(id,origin_key,monitor,revision,started_at,deadline,payload) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(origin_key) DO NOTHING`, r.ID, key, r.Monitor, r.Revision, r.StartedAt, r.Deadline, b)
	if err != nil {
		return r, err
	}
	err = s.pool.QueryRow(ctx, `SELECT payload,state,used FROM nmo_fallback_runs WHERE origin_key=$1`, key).Scan(&b, &r.State, &r.Used)
	state, used := r.State, r.Used
	if err == nil {
		err = json.Unmarshal(b, &r)
		r.State = state
		r.Used = used
	}
	return r, err
}

func (s *Store) ReserveFallback(ctx context.Context, t DiagnosticTask) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var b []byte
	var used int
	var state string
	if err = tx.QueryRow(ctx, `SELECT payload,used,state FROM nmo_fallback_runs WHERE id=$1 FOR UPDATE`, t.RunID).Scan(&b, &used, &state); err != nil {
		return err
	}
	var r FallbackRun
	if err = json.Unmarshal(b, &r); err != nil {
		return err
	}
	if state != "pending" || used >= r.MaxAttempts || t.AttemptNo != used+1 || t.Plan.Name != r.Monitor || t.Plan.Revision != r.Revision || t.Check.ID != r.Check || !time.Now().Before(t.Deadline) || t.Deadline.After(r.Deadline) {
		return fmt.Errorf("fallback reservation fenced")
	}
	var regular monitor.Check
	for _, c := range t.Plan.Checks {
		if c.ID == r.Check {
			regular = c
		}
	}
	want, _ := json.Marshal(config.FallbackCheck(regular))
	actual, _ := json.Marshal(t.Check)
	if regular.Fallback == nil || regular.Fallback.MaxAttempts != r.MaxAttempts || regular.Fallback.Strategy != r.Strategy || !slices.Contains(regular.Fallback.On, r.Cause) {
		return fmt.Errorf("fallback policy mismatch")
	}
	var latestID string
	var latestAt time.Time
	err = tx.QueryRow(ctx, `SELECT id,finished_at FROM nmo_observations WHERE agent_id=$1 AND monitor=$2 AND revision=$3 AND check_id=$4 AND diagnostic_task_id='' AND fallback_task_id='' ORDER BY finished_at DESC,id DESC LIMIT 1`, r.OriginAgent, r.Monitor, r.Revision, r.Check).Scan(&latestID, &latestAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if r.OriginID != "" {
		if latestID != r.OriginID || time.Since(r.StartedAt) > t.Plan.MaxAge {
			return fmt.Errorf("fallback origin superseded or expired")
		}
		var original []byte
		if err = tx.QueryRow(ctx, `SELECT payload FROM nmo_observations WHERE id=$1`, r.OriginID).Scan(&original); err != nil {
			return err
		}
		var o monitor.Observation
		if err = json.Unmarshal(original, &o); err != nil {
			return err
		}
		if monitor.FallbackCause(o) != r.Cause {
			return fmt.Errorf("fallback origin is not technical failure")
		}
	} else if (r.Cause != "agent_unreachable" && r.Cause != "config_rejected") || (!latestAt.IsZero() && !latestAt.Before(r.StartedAt)) {
		return fmt.Errorf("fallback missing origin not justified")
	}
	if r.OriginID == "" && r.Cause == "config_rejected" {
		var rejected bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_config_rejections WHERE agent_id=$1 AND revision=$2)`, r.OriginAgent, r.Revision).Scan(&rejected); err != nil {
			return err
		}
		if !rejected {
			return fmt.Errorf("missing config rejection proof")
		}
	}
	if string(want) != string(actual) || regular.Fallback == nil || !t.Plan.FallbackCandidate(regular, monitor.Slot{Agent: r.OriginAgent, Zone: r.OriginZone}, monitor.Slot{Agent: t.Agent, Zone: t.Zone}, t.Plan.FallbackDomains) {
		return fmt.Errorf("fallback candidate/check denied")
	}
	var active, seen bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_fallback_tasks WHERE run_id=$1 AND state IN ('reserved','accepted') AND deadline>clock_timestamp()),EXISTS(SELECT 1 FROM nmo_fallback_tasks WHERE run_id=$1 AND agent_id=$2)`, r.ID, t.Agent).Scan(&active, &seen); err != nil {
		return err
	}
	if active || seen {
		return fmt.Errorf("fallback active or candidate already attempted")
	}
	var gen int64
	var applied bool
	if err = tx.QueryRow(ctx, `SELECT generation,EXISTS(SELECT 1 FROM nmo_session_revisions WHERE agent_id=$1 AND generation=nmo_sessions.generation AND revision=$2) FROM nmo_sessions WHERE agent_id=$1 FOR UPDATE`, t.Agent, t.Plan.Revision).Scan(&gen, &applied); err != nil {
		return err
	}
	if gen != t.Generation || !applied {
		return ErrFenced
	}
	_, err = tx.Exec(ctx, `INSERT INTO nmo_fallback_tasks(id,run_id,agent_id,zone,monitor,revision,check_id,deadline,attempt_no) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, t.ID, r.ID, t.Agent, t.Zone, r.Monitor, r.Revision, r.Check, t.Deadline, t.AttemptNo)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE nmo_fallback_runs SET used=used+1 WHERE id=$1`, r.ID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateFallbackObservation(ctx context.Context, tx pgx.Tx, o monitor.Observation) error {
	var check, agent, zone, name, revision, state string
	var deadline time.Time
	err := tx.QueryRow(ctx, `SELECT check_id,agent_id,zone,monitor,revision,state,deadline FROM nmo_fallback_tasks WHERE id=$1 FOR UPDATE`, o.FallbackTaskID).Scan(&check, &agent, &zone, &name, &revision, &state, &deadline)
	if err != nil {
		return err
	}
	if o.DiagnosticTaskID != "" || o.ID != o.FallbackTaskID || o.Check != check || o.Agent != agent || o.Zone != zone || o.Monitor != name || o.Revision != revision || state == "rejected" || o.StartedAt.After(deadline) || o.FinishedAt.After(deadline.Add(5*time.Second)) {
		return fmt.Errorf("fallback result does not match reservation")
	}
	_, err = tx.Exec(ctx, `UPDATE nmo_fallback_tasks SET state='finished' WHERE id=$1`, o.ID)
	return err
}

func (s *Store) FallbackReceipt(ctx context.Context, id, agent string, generation int64, accepted bool) (bool, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_fallback_tasks WHERE id=$1)`, id).Scan(&exists); err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	state := "rejected"
	if accepted {
		state = "accepted"
	}
	tag, err := s.pool.Exec(ctx, `UPDATE nmo_fallback_tasks SET state=$3 WHERE id=$1 AND agent_id=$2 AND state='reserved' AND EXISTS(SELECT 1 FROM nmo_sessions WHERE agent_id=$2 AND generation=$4)`, id, agent, state, generation)
	if err == nil && tag.RowsAffected() != 1 {
		err = fmt.Errorf("fallback receipt fenced")
	}
	return true, err
}

func (s *Store) FallbackTasks(ctx context.Context, run string) ([]FallbackTask, error) {
	rows, err := s.pool.Query(ctx, `SELECT t.id,t.agent_id,t.zone,t.attempt_no,t.state,t.deadline,COALESCE(o.id,'') FROM nmo_fallback_tasks t LEFT JOIN nmo_observations o ON o.fallback_task_id=t.id WHERE run_id=$1 ORDER BY attempt_no LIMIT 3`, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []FallbackTask{}
	for rows.Next() {
		var t FallbackTask
		if err = rows.Scan(&t.ID, &t.Agent, &t.Zone, &t.Attempt, &t.State, &t.Deadline, &t.ObservationID); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
func (s *Store) CloseFallback(ctx context.Context, id, state string) error {
	_, err := s.pool.Exec(ctx, `UPDATE nmo_fallback_runs SET state=$2 WHERE id=$1 AND state='pending'`, id, state)
	return err
}

// ExpireFallbacks bounds each sweep; admission never replenishes these roots.
func (s *Store) ExpireFallbacks(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE nmo_fallback_runs SET state='expired' WHERE id IN (SELECT id FROM nmo_fallback_runs WHERE state='pending' AND deadline<clock_timestamp() ORDER BY deadline LIMIT 200)`)
	return err
}

func (s *Store) FallbackRuns(ctx context.Context, monitorName, before string, limit int) ([]FallbackRun, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("invalid fallback limit")
	}
	rows, err := s.pool.Query(ctx, `SELECT payload,state,used FROM nmo_fallback_runs WHERE ($1='' OR monitor=$1) AND ($2='' OR (started_at,id)<(SELECT started_at,id FROM nmo_fallback_runs WHERE id=$2)) ORDER BY started_at DESC,id DESC LIMIT $3`, monitorName, before, limit+1)
	if err != nil {
		return nil, err
	}
	var runs []FallbackRun
	for rows.Next() {
		var b []byte
		var state string
		var used int
		if err = rows.Scan(&b, &state, &used); err != nil {
			rows.Close()
			return nil, err
		}
		var r FallbackRun
		if err = json.Unmarshal(b, &r); err != nil {
			rows.Close()
			return nil, err
		}
		r.State = state
		r.Used = used
		if state == "pending" && time.Now().After(r.Deadline) {
			r.State = "expired"
		}
		runs = append(runs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range runs {
		runs[i].Tasks, err = s.FallbackTasks(ctx, runs[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return runs, nil
}

func (s *Store) ReadFallback(ctx context.Context, id string) (FallbackRun, bool, error) {
	var b []byte
	var state string
	var used int
	var r FallbackRun
	err := s.pool.QueryRow(ctx, `SELECT payload,state,used FROM nmo_fallback_runs WHERE id=$1`, id).Scan(&b, &state, &used)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, false, nil
	}
	if err != nil {
		return r, false, err
	}
	if err = json.Unmarshal(b, &r); err != nil {
		return r, false, err
	}
	r.State = state
	r.Used = used
	if state == "pending" && time.Now().After(r.Deadline) {
		r.State = "expired"
	}
	r.Tasks, err = s.FallbackTasks(ctx, id)
	return r, true, err
}
