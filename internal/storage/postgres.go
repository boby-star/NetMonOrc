package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFenced = errors.New("session superseded")

type Store struct {
	pool       *pgxpool.Pool
	controller *pgxpool.Conn
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	c, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		return nil, errors.New("invalid database configuration")
	}
	c.MaxConns = 4
	p, e := pgxpool.NewWithConfig(ctx, c)
	if e != nil {
		return nil, errors.New("database initialization failed")
	}
	s := &Store{pool: p}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, fmt.Errorf("database unavailable: %w", e)
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		p.Close()
		return nil, e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781301);
CREATE TABLE IF NOT EXISTS nmo_sessions(agent_id text PRIMARY KEY,generation bigint NOT NULL,applied_revision text NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS nmo_assignments(agent_id text NOT NULL,revision text NOT NULL,zone text NOT NULL,monitor text NOT NULL,PRIMARY KEY(agent_id,revision,zone,monitor));
CREATE TABLE IF NOT EXISTS nmo_observations(id text PRIMARY KEY,agent_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,check_id text NOT NULL,finished_at timestamptz NOT NULL,received_at timestamptz NOT NULL DEFAULT now(),payload jsonb NOT NULL,payload_hash bytea NOT NULL);
CREATE INDEX IF NOT EXISTS nmo_observations_live ON nmo_observations(monitor,revision,agent_id,check_id,finished_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS nmo_assessments(monitor text PRIMARY KEY,revision text NOT NULL,evaluated_at timestamptz NOT NULL,payload jsonb NOT NULL);
ALTER TABLE nmo_observations ADD COLUMN IF NOT EXISTS diagnostic_task_id text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS nmo_incidents(monitor text PRIMARY KEY,episode_id text NOT NULL DEFAULT '',run_id text NOT NULL DEFAULT '',last_finished timestamptz);
CREATE TABLE IF NOT EXISTS nmo_diagnostic_runs(id text PRIMARY KEY,episode_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,profile_hash text NOT NULL,mode text NOT NULL,max_checks integer NOT NULL,used_checks integer NOT NULL DEFAULT 0,deadline timestamptz NOT NULL,state text NOT NULL,reason text NOT NULL DEFAULT '',report jsonb,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz);
CREATE INDEX IF NOT EXISTS nmo_diagnostic_runs_monitor ON nmo_diagnostic_runs(monitor,started_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS nmo_diagnostic_tasks(id text PRIMARY KEY,run_id text NOT NULL REFERENCES nmo_diagnostic_runs(id),agent_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,zone text NOT NULL,check_id text NOT NULL,deadline timestamptz NOT NULL,check_spec jsonb NOT NULL,state text NOT NULL);`)
	if e == nil {
		e = tx.Commit(ctx)
	}
	if e != nil {
		p.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() {
	if s.controller != nil {
		s.controller.Conn().Close(context.Background())
		s.controller.Release()
	}
	s.pool.Close()
}
func (s *Store) BeginSession(ctx context.Context, agent string) (int64, error) {
	var n int64
	e := s.pool.QueryRow(ctx, `INSERT INTO nmo_sessions(agent_id,generation) VALUES($1,1) ON CONFLICT(agent_id) DO UPDATE SET generation=nmo_sessions.generation+1,applied_revision='' RETURNING generation`, agent).Scan(&n)
	return n, e
}

// Historical assignment bindings permit old-revision replay without trusting
// a zone newly claimed by an Agent after reconnect or relocation.
func (s *Store) Assign(ctx context.Context, agent string, generation int64, revision, zone, monitorName string) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var n int64
	if e = tx.QueryRow(ctx, `SELECT generation FROM nmo_sessions WHERE agent_id=$1 FOR UPDATE`, agent).Scan(&n); e != nil {
		return e
	}
	if n != generation {
		return ErrFenced
	}
	if _, e = tx.Exec(ctx, `INSERT INTO nmo_assignments(agent_id,revision,zone,monitor) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, agent, revision, zone, monitorName); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) Applied(ctx context.Context, agent string, generation int64, revision string) error {
	t, e := s.pool.Exec(ctx, `UPDATE nmo_sessions SET applied_revision=$3 WHERE agent_id=$1 AND generation=$2`, agent, generation, revision)
	if e != nil {
		return e
	}
	if t.RowsAffected() != 1 {
		return ErrFenced
	}
	return nil
}

// CommitBatch locks the generation until COMMIT. A reconnect either precedes
// this transaction or fences it; an ACK is never emitted for uncommitted data.
func (s *Store) CommitBatch(ctx context.Context, agent string, generation int64, appliedRevision string, observations []monitor.Observation) ([]bool, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var n int64
	var applied string
	e = tx.QueryRow(ctx, `SELECT generation,applied_revision FROM nmo_sessions WHERE agent_id=$1 FOR UPDATE`, agent).Scan(&n, &applied)
	if e != nil {
		return nil, e
	}
	if n != generation || applied != appliedRevision {
		return nil, ErrFenced
	}
	duplicates := make([]bool, len(observations))
	for i, o := range observations {
		var assigned bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_assignments WHERE agent_id=$1 AND revision=$2 AND zone=$3 AND monitor=$4)`, agent, o.Revision, o.Zone, o.Monitor).Scan(&assigned); e != nil {
			return nil, e
		}
		if !assigned {
			return nil, fmt.Errorf("observation has no historical assignment binding")
		}
		if o.DiagnosticTaskID != "" {
			if e = validateDiagnosticObservation(ctx, tx, o); e != nil {
				return nil, e
			}
		}
		b, e := json.Marshal(o)
		if e != nil {
			return nil, e
		}
		hash := sha256.Sum256(b)
		t, e := tx.Exec(ctx, `INSERT INTO nmo_observations(id,agent_id,monitor,revision,check_id,finished_at,payload,payload_hash,diagnostic_task_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT(id) DO NOTHING`, o.ID, agent, o.Monitor, o.Revision, o.Check, o.FinishedAt, b, hash[:], o.DiagnosticTaskID)
		if e != nil {
			return nil, e
		}
		if t.RowsAffected() == 0 {
			var previous []byte
			e = tx.QueryRow(ctx, `SELECT payload_hash FROM nmo_observations WHERE id=$1`, o.ID).Scan(&previous)
			if e != nil {
				return nil, e
			}
			if !bytes.Equal(previous, hash[:]) {
				return nil, fmt.Errorf("observation ID conflict")
			}
			duplicates[i] = true
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return duplicates, nil
}
func (s *Store) Latest(ctx context.Context, p monitor.Plan, now time.Time) ([]monitor.Observation, error) {
	rows, e := s.pool.Query(ctx, `SELECT payload FROM (SELECT DISTINCT ON(agent_id,check_id) payload,agent_id,check_id FROM nmo_observations WHERE monitor=$1 AND revision=$2 AND diagnostic_task_id='' AND finished_at >= $3 ORDER BY agent_id,check_id,finished_at DESC,id DESC) x LIMIT 10001`, p.Name, p.Revision, now.Add(-p.MaxAge))
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []monitor.Observation
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var o monitor.Observation
		if e = json.Unmarshal(b, &o); e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	if len(out) > 10000 {
		return nil, fmt.Errorf("live observation bound exceeded")
	}
	return out, rows.Err()
}
func (s *Store) SaveAssessment(ctx context.Context, a monitor.Assessment) error {
	b, e := json.Marshal(a)
	if e != nil {
		return e
	}
	_, e = s.pool.Exec(ctx, `INSERT INTO nmo_assessments(monitor,revision,evaluated_at,payload) VALUES($1,$2,$3,$4) ON CONFLICT(monitor) DO UPDATE SET revision=EXCLUDED.revision,evaluated_at=EXCLUDED.evaluated_at,payload=EXCLUDED.payload`, a.Monitor, a.Revision, a.At, b)
	return e
}
