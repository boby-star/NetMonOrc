package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/jackc/pgx/v5"
)

func newID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}

// LockController excludes a second operational diagnostic coordinator. The
// dedicated DB connection is retained until Close; run rows also fence writes.
func (s *Store) LockController(ctx context.Context) error {
	c, e := s.pool.Acquire(ctx)
	if e != nil {
		return e
	}
	var ok bool
	e = c.QueryRow(ctx, `SELECT pg_try_advisory_lock(781302)`).Scan(&ok)
	if e != nil || !ok {
		c.Release()
		return fmt.Errorf("another diagnostic Controller is active")
	}
	s.controller = c
	_, e = c.Exec(ctx, `UPDATE nmo_diagnostic_runs SET state='inconclusive',reason='controller_restart',finished_at=now() WHERE state='running';
 UPDATE nmo_incidents i SET last_finished=r.finished_at FROM nmo_diagnostic_runs r WHERE i.run_id=r.id AND r.reason='controller_restart'`)
	return e
}
func (s *Store) ControllerAlive(ctx context.Context) error {
	if s.controller == nil {
		return fmt.Errorf("Controller lock unavailable")
	}
	_, e := s.controller.Exec(ctx, "SELECT 1")
	return e
}

// StartDiagnostic admits one session per incident. Reconnects, new revisions
// and terminal sessions keep the same incident until healthy/full coverage.
func (s *Store) StartDiagnostic(ctx context.Context, p monitor.Plan, profile diagnostics.Profile) (string, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `INSERT INTO nmo_incidents(monitor) VALUES($1) ON CONFLICT DO NOTHING`, p.Name)
	if e != nil {
		return "", e
	}
	var episode, run string
	var last *time.Time
	e = tx.QueryRow(ctx, `SELECT episode_id,run_id,last_finished FROM nmo_incidents WHERE monitor=$1 FOR UPDATE`, p.Name).Scan(&episode, &run, &last)
	if e != nil {
		return "", e
	}
	if run != "" || (last != nil && time.Since(*last) < p.Diagnostics.Cooldown) {
		return "", nil
	}
	if episode == "" {
		episode = newID()
	}
	run = newID()
	if episode == "" || run == "" {
		return "", fmt.Errorf("random identity unavailable")
	}
	_, e = tx.Exec(ctx, `INSERT INTO nmo_diagnostic_runs(id,episode_id,monitor,revision,profile_hash,mode,max_checks,deadline,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'running')`, run, episode, p.Name, p.Revision, p.Diagnostics.ProfileHash, profile.Mode, profile.Budget.MaxChecks, time.Now().Add(profile.Budget.MaxDuration))
	if e != nil {
		return "", e
	}
	_, e = tx.Exec(ctx, `UPDATE nmo_incidents SET episode_id=$2,run_id=$3 WHERE monitor=$1`, p.Name, episode, run)
	if e != nil {
		return "", e
	}
	return run, tx.Commit(ctx)
}
func (s *Store) CloseDiagnosticEpisode(ctx context.Context, a monitor.Assessment) error {
	if a.State != "healthy" || len(a.Zones) == 0 {
		return nil
	}
	for _, z := range a.Zones {
		if z.Coverage != "complete" {
			return nil
		}
	}
	_, e := s.pool.Exec(ctx, `UPDATE nmo_incidents i SET episode_id='',run_id='' WHERE monitor=$1 AND NOT EXISTS(SELECT 1 FROM nmo_diagnostic_runs r WHERE r.id=i.run_id AND r.state='running')`, a.Monitor)
	return e
}
func (s *Store) FinishDiagnostic(ctx context.Context, id string, r diagnostics.Result) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, `UPDATE nmo_diagnostic_runs SET state=$2,reason=$3,report=$4,finished_at=now() WHERE id=$1 AND state='running'`, id, r.Status, r.Reason, b)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("diagnostic run is fenced")
	}
	_, e = tx.Exec(ctx, `UPDATE nmo_incidents SET last_finished=now() WHERE run_id=$1`, id)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type DiagnosticTask struct {
	ID         string
	RunID      string
	Agent      string
	Generation int64
	Plan       monitor.Plan
	Check      monitor.Check
	Zone       string
	Deadline   time.Time
}

// A reservation is consumed before sending. A lost frame or restart never
// refunds it or silently launches a new physical attempt under the same ID.
func (s *Store) ReserveDiagnostic(ctx context.Context, t DiagnosticTask) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var used, limit int
	var state, revision string
	var deadline time.Time
	e = tx.QueryRow(ctx, `SELECT used_checks,max_checks,state,revision,deadline FROM nmo_diagnostic_runs WHERE id=$1 FOR UPDATE`, t.RunID).Scan(&used, &limit, &state, &revision, &deadline)
	if e != nil {
		return e
	}
	if state != "running" || used >= limit || revision != t.Plan.Revision || !time.Now().Before(deadline) || t.Deadline.After(deadline) || !time.Now().Before(t.Deadline) {
		return fmt.Errorf("diagnostic run budget or deadline unavailable")
	}
	var gen int64
	var applied string
	e = tx.QueryRow(ctx, `SELECT generation,applied_revision FROM nmo_sessions WHERE agent_id=$1 FOR UPDATE`, t.Agent).Scan(&gen, &applied)
	if e != nil {
		return e
	}
	if gen != t.Generation || applied != t.Plan.Revision {
		return ErrFenced
	}
	b, e := json.Marshal(t.Check)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO nmo_diagnostic_tasks(id,run_id,agent_id,monitor,revision,zone,check_id,deadline,check_spec,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'reserved')`, t.ID, t.RunID, t.Agent, t.Plan.Name, t.Plan.Revision, t.Zone, t.Check.ID, t.Deadline, b)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `UPDATE nmo_diagnostic_runs SET used_checks=used_checks+1 WHERE id=$1`, t.RunID)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) DiagnosticReceipt(ctx context.Context, task, agent string, generation int64, accepted bool) error {
	state := "rejected"
	if accepted {
		state = "accepted"
	}
	tag, e := s.pool.Exec(ctx, `UPDATE nmo_diagnostic_tasks SET state=$3 WHERE id=$1 AND agent_id=$2 AND state='reserved' AND EXISTS(SELECT 1 FROM nmo_sessions WHERE agent_id=$2 AND generation=$4)`, task, agent, state, generation)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("unexpected task receipt")
	}
	return nil
}
func validateDiagnosticObservation(ctx context.Context, tx pgx.Tx, o monitor.Observation) error {
	var check, agent, zone, revision, name, state string
	var deadline time.Time
	e := tx.QueryRow(ctx, `SELECT check_id,agent_id,zone,revision,monitor,state,deadline FROM nmo_diagnostic_tasks WHERE id=$1 FOR UPDATE`, o.DiagnosticTaskID).Scan(&check, &agent, &zone, &revision, &name, &state, &deadline)
	if e != nil {
		return e
	}
	if o.ID != o.DiagnosticTaskID || check != o.Check || agent != o.Agent || zone != o.Zone || revision != o.Revision || name != o.Monitor || state == "rejected" || o.StartedAt.After(deadline) || o.FinishedAt.After(deadline.Add(5*time.Second)) {
		return fmt.Errorf("diagnostic result does not match its reservation")
	}
	_, e = tx.Exec(ctx, `UPDATE nmo_diagnostic_tasks SET state='finished' WHERE id=$1`, o.DiagnosticTaskID)
	return e
}

// DiagnosticReport retains the difference between reserved budget and known
// planner outcomes. After a crash, physical completion may be unknown.
type DiagnosticReport struct {
	ID             string             `json:"id"`
	Monitor        string             `json:"monitor"`
	Revision       string             `json:"revision"`
	ReservedChecks int                `json:"reservedChecks"`
	StartedAt      time.Time          `json:"startedAt"`
	FinishedAt     *time.Time         `json:"finishedAt,omitempty"`
	Result         diagnostics.Result `json:"result"`
}

func (s *Store) LatestDiagnostic(ctx context.Context, monitorName string) (*DiagnosticReport, error) {
	var r DiagnosticReport
	var b []byte
	var state, reason, mode string
	e := s.pool.QueryRow(ctx, `SELECT id,monitor,revision,used_checks,started_at,finished_at,state,reason,mode,report FROM nmo_diagnostic_runs WHERE monitor=$1 ORDER BY started_at DESC,id DESC LIMIT 1`, monitorName).Scan(&r.ID, &r.Monitor, &r.Revision, &r.ReservedChecks, &r.StartedAt, &r.FinishedAt, &state, &reason, &mode, &b)
	if e == pgx.ErrNoRows {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if len(b) > 0 {
		if e = json.Unmarshal(b, &r.Result); e != nil {
			return nil, e
		}
	} else {
		r.Result = diagnostics.Result{Mode: mode, Status: state, Reason: reason, Hypotheses: []string{}, Decisions: []diagnostics.Decision{}}
	}
	return &r, nil
}
