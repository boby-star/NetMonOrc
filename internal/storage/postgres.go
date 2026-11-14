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
CREATE TABLE IF NOT EXISTS nmo_resources(kind text NOT NULL DEFAULT 'Monitor',name text NOT NULL,uid text NOT NULL UNIQUE,version bigint NOT NULL DEFAULT 1,generation bigint NOT NULL DEFAULT 1,manifest jsonb NOT NULL,effective_revision text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(kind,name));
CREATE TABLE IF NOT EXISTS nmo_resource_bootstrap(kind text NOT NULL,name text NOT NULL,PRIMARY KEY(kind,name));
CREATE TABLE IF NOT EXISTS nmo_resource_revisions(uid text NOT NULL,version bigint NOT NULL,generation bigint NOT NULL,manifest jsonb NOT NULL,changed_at timestamptz NOT NULL DEFAULT now(),actor text NOT NULL,PRIMARY KEY(uid,version));
CREATE TABLE IF NOT EXISTS nmo_operations(id text PRIMARY KEY,actor_id text NOT NULL,request_key text,request_hash bytea NOT NULL,idem_expires_at timestamptz NOT NULL,state text NOT NULL,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,updated_at timestamptz NOT NULL DEFAULT now(),payload jsonb NOT NULL);
CREATE UNIQUE INDEX IF NOT EXISTS nmo_operation_request ON nmo_operations(actor_id,request_key) WHERE request_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS nmo_operation_pending ON nmo_operations(updated_at,id) WHERE state IN ('accepted','running');
CREATE TABLE IF NOT EXISTS nmo_users(id text PRIMARY KEY,username text UNIQUE NOT NULL,password_hash text NOT NULL,role text NOT NULL CHECK(role IN ('admin','viewer')),disabled boolean NOT NULL DEFAULT false,failed_logins integer NOT NULL DEFAULT 0,locked_until timestamptz);
ALTER TABLE nmo_users ADD COLUMN IF NOT EXISTS created_at timestamptz;
ALTER TABLE nmo_users ALTER COLUMN created_at SET DEFAULT now();
ALTER TABLE nmo_users ADD COLUMN IF NOT EXISTS version bigint NOT NULL DEFAULT 1;
CREATE TABLE IF NOT EXISTS nmo_api_tokens(id text PRIMARY KEY,user_id text NOT NULL REFERENCES nmo_users(id) ON DELETE CASCADE,name text NOT NULL,token_hash bytea NOT NULL UNIQUE,scopes text[] NOT NULL,expires_at timestamptz,created_at timestamptz NOT NULL DEFAULT now(),revoked_at timestamptz);
CREATE INDEX IF NOT EXISTS nmo_api_tokens_owner ON nmo_api_tokens(user_id,id DESC);
CREATE TABLE IF NOT EXISTS nmo_browser_sessions(token_hash bytea PRIMARY KEY,user_id text NOT NULL REFERENCES nmo_users(id) ON DELETE CASCADE,expires_at timestamptz NOT NULL);
CREATE INDEX IF NOT EXISTS nmo_browser_session_expiry ON nmo_browser_sessions(expires_at);
CREATE TABLE IF NOT EXISTS nmo_identity_audit(id bigserial PRIMARY KEY,username text NOT NULL,action text NOT NULL,succeeded boolean NOT NULL,occurred_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS nmo_sessions(agent_id text PRIMARY KEY,generation bigint NOT NULL,applied_revision text NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS nmo_assignments(agent_id text NOT NULL,revision text NOT NULL,zone text NOT NULL,monitor text NOT NULL,PRIMARY KEY(agent_id,revision,zone,monitor));
CREATE TABLE IF NOT EXISTS nmo_observations(id text PRIMARY KEY,agent_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,check_id text NOT NULL,finished_at timestamptz NOT NULL,received_at timestamptz NOT NULL DEFAULT now(),payload jsonb NOT NULL,payload_hash bytea NOT NULL);
CREATE INDEX IF NOT EXISTS nmo_observations_history ON nmo_observations(monitor,finished_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS nmo_observations_live ON nmo_observations(monitor,revision,agent_id,check_id,finished_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS nmo_assessments(monitor text PRIMARY KEY,revision text NOT NULL,evaluated_at timestamptz NOT NULL,payload jsonb NOT NULL);
ALTER TABLE nmo_observations ADD COLUMN IF NOT EXISTS diagnostic_task_id text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS nmo_incidents(monitor text PRIMARY KEY,episode_id text NOT NULL DEFAULT '',run_id text NOT NULL DEFAULT '',last_finished timestamptz);
CREATE TABLE IF NOT EXISTS nmo_diagnostic_runs(id text PRIMARY KEY,episode_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,profile_hash text NOT NULL,mode text NOT NULL,max_checks integer NOT NULL,used_checks integer NOT NULL DEFAULT 0,deadline timestamptz NOT NULL,state text NOT NULL,reason text NOT NULL DEFAULT '',report jsonb,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz);
ALTER TABLE nmo_diagnostic_runs ADD COLUMN IF NOT EXISTS profile_ref text NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS nmo_diagnostic_runs_history ON nmo_diagnostic_runs(started_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS nmo_diagnostic_runs_monitor ON nmo_diagnostic_runs(monitor,started_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS nmo_alerts(logical_key text PRIMARY KEY,payload jsonb NOT NULL);
CREATE TABLE IF NOT EXISTS nmo_alert_events(id text PRIMARY KEY,logical_key text NOT NULL,transition jsonb NOT NULL,suppressed boolean NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS nmo_webhook_outbox(event_id text PRIMARY KEY REFERENCES nmo_alert_events(id),endpoint_hash text NOT NULL,payload jsonb NOT NULL,state text NOT NULL DEFAULT 'pending',attempts integer NOT NULL DEFAULT 0,next_after timestamptz NOT NULL,expires_at timestamptz NOT NULL,lease_token text NOT NULL DEFAULT '',lease_until timestamptz,status_code integer NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS nmo_webhook_due ON nmo_webhook_outbox(state,next_after);
CREATE TABLE IF NOT EXISTS nmo_webhook_attempts(event_id text NOT NULL REFERENCES nmo_webhook_outbox(event_id),attempt integer NOT NULL,lease_token text NOT NULL,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,status_code integer NOT NULL DEFAULT 0,PRIMARY KEY(event_id,attempt));
CREATE TABLE IF NOT EXISTS nmo_diagnostic_tasks(id text PRIMARY KEY,run_id text NOT NULL REFERENCES nmo_diagnostic_runs(id),agent_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,zone text NOT NULL,check_id text NOT NULL,deadline timestamptz NOT NULL,check_spec jsonb NOT NULL,state text NOT NULL);
CREATE INDEX IF NOT EXISTS nmo_diagnostic_tasks_run ON nmo_diagnostic_tasks(run_id,id);
CREATE INDEX IF NOT EXISTS nmo_observations_diagnostic_task ON nmo_observations(diagnostic_task_id,id) WHERE diagnostic_task_id<>'';
ALTER TABLE nmo_diagnostic_tasks ADD COLUMN IF NOT EXISTS attempt_no integer NOT NULL DEFAULT 1;`)
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
