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
	"github.com/boby-star/NetMonOrc/internal/webhook"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrFenced = errors.New("session superseded")

type Store struct {
	pool             *pgxpool.Pool
	managedWebhooks  bool
	webhookPreflight func(webhook.Endpoint) error
	controller       *pgxpool.Conn
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	return openStore(ctx, dsn, false)
}

// OpenAdmin reuses an initialized schema without taking DDL locks against the
// running Controller. A fresh database still receives the normal migrations.
func OpenAdmin(ctx context.Context, dsn string) (*Store, error) {
	return openStore(ctx, dsn, true)
}

func openStore(ctx context.Context, dsn string, reuseSchema bool) (*Store, error) {
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
	if reuseSchema {
		var initialized bool
		if e = p.QueryRow(ctx, `SELECT to_regclass('nmo_users') IS NOT NULL AND to_regclass('nmo_identity_audit') IS NOT NULL`).Scan(&initialized); e != nil {
			p.Close()
			return nil, e
		}
		if initialized {
			return s, nil
		}
	}
	tx, e := p.Begin(ctx)
	if e != nil {
		p.Close()
		return nil, e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781301);
CREATE TABLE IF NOT EXISTS nmo_resources(kind text NOT NULL DEFAULT 'Monitor',name text NOT NULL,uid text NOT NULL UNIQUE,version bigint NOT NULL DEFAULT 1,generation bigint NOT NULL DEFAULT 1,manifest jsonb NOT NULL,effective_revision text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(kind,name));
CREATE TABLE IF NOT EXISTS nmo_regular_placements(monitor_uid text PRIMARY KEY REFERENCES nmo_resources(uid) ON DELETE CASCADE,policy_hash text NOT NULL,selected jsonb NOT NULL);
ALTER TABLE nmo_regular_placements ADD COLUMN IF NOT EXISTS plan_hash text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS nmo_check_placements(monitor_uid text NOT NULL REFERENCES nmo_resources(uid) ON DELETE CASCADE,check_id text NOT NULL,policy_hash text NOT NULL,selected jsonb NOT NULL,plan_hash text NOT NULL,PRIMARY KEY(monitor_uid,check_id));
CREATE TABLE IF NOT EXISTS nmo_placement_transitions(resource_uid text NOT NULL REFERENCES nmo_resources(uid) ON DELETE CASCADE,from_revision text NOT NULL,to_revision text NOT NULL,PRIMARY KEY(resource_uid,from_revision,to_revision));
CREATE TABLE IF NOT EXISTS nmo_placement_cursors(policy_key text PRIMARY KEY,next_offset bigint NOT NULL DEFAULT 0 CHECK(next_offset>=0));
CREATE TABLE IF NOT EXISTS nmo_resource_bootstrap(kind text NOT NULL,name text NOT NULL,PRIMARY KEY(kind,name));
CREATE TABLE IF NOT EXISTS nmo_resource_revisions(uid text NOT NULL,version bigint NOT NULL,generation bigint NOT NULL,manifest jsonb NOT NULL,changed_at timestamptz NOT NULL DEFAULT now(),actor text NOT NULL,PRIMARY KEY(uid,version));
CREATE TABLE IF NOT EXISTS nmo_operations(id text PRIMARY KEY,actor_id text NOT NULL,request_key text,request_hash bytea NOT NULL,idem_expires_at timestamptz NOT NULL,state text NOT NULL,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,updated_at timestamptz NOT NULL DEFAULT now(),payload jsonb NOT NULL);
ALTER TABLE nmo_operations ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'config.apply';
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
CREATE TABLE IF NOT EXISTS nmo_response_episodes(monitor text NOT NULL,revision text NOT NULL,rule_id text NOT NULL,fired boolean NOT NULL DEFAULT false,PRIMARY KEY(monitor,revision,rule_id));
CREATE TABLE IF NOT EXISTS nmo_responses(id text PRIMARY KEY,monitor text NOT NULL,revision text NOT NULL,rule_id text NOT NULL,agent text NOT NULL,generation bigint NOT NULL,status text NOT NULL,reason text NOT NULL DEFAULT '',created_at timestamptz NOT NULL,accept_until timestamptz NOT NULL,block_until timestamptz NOT NULL,assessment jsonb NOT NULL,rule jsonb NOT NULL);
CREATE INDEX IF NOT EXISTS nmo_responses_monitor ON nmo_responses(monitor,created_at DESC);
CREATE TABLE IF NOT EXISTS nmo_session_revisions(agent_id text NOT NULL REFERENCES nmo_sessions(agent_id) ON DELETE CASCADE,revision text NOT NULL,generation bigint NOT NULL,PRIMARY KEY(agent_id,revision));
CREATE TABLE IF NOT EXISTS nmo_assignments(agent_id text NOT NULL,revision text NOT NULL,zone text NOT NULL,monitor text NOT NULL,PRIMARY KEY(agent_id,revision,zone,monitor));
ALTER TABLE nmo_assignments ADD COLUMN IF NOT EXISTS regular_enabled boolean NOT NULL DEFAULT true;
ALTER TABLE nmo_assignments ADD COLUMN IF NOT EXISTS allowed_checks text[];
CREATE TABLE IF NOT EXISTS nmo_observations(id text PRIMARY KEY,agent_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,check_id text NOT NULL,finished_at timestamptz NOT NULL,received_at timestamptz NOT NULL DEFAULT now(),payload jsonb NOT NULL,payload_hash bytea NOT NULL);
CREATE INDEX IF NOT EXISTS nmo_observations_history ON nmo_observations(monitor,finished_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS nmo_observations_live ON nmo_observations(monitor,revision,agent_id,check_id,finished_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS nmo_assessments(monitor text PRIMARY KEY,revision text NOT NULL,evaluated_at timestamptz NOT NULL,payload jsonb NOT NULL);
ALTER TABLE nmo_observations ADD COLUMN IF NOT EXISTS diagnostic_task_id text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS nmo_incidents(monitor text PRIMARY KEY,episode_id text NOT NULL DEFAULT '',run_id text NOT NULL DEFAULT '',last_finished timestamptz);
CREATE TABLE IF NOT EXISTS nmo_diagnostic_runs(id text PRIMARY KEY,episode_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,profile_hash text NOT NULL,mode text NOT NULL,max_checks integer NOT NULL,used_checks integer NOT NULL DEFAULT 0,deadline timestamptz NOT NULL,state text NOT NULL,reason text NOT NULL DEFAULT '',report jsonb,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz);
ALTER TABLE nmo_diagnostic_runs ADD COLUMN IF NOT EXISTS max_parallel integer NOT NULL DEFAULT 1;
ALTER TABLE nmo_diagnostic_runs ADD COLUMN IF NOT EXISTS max_estimated_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE nmo_diagnostic_runs ADD COLUMN IF NOT EXISTS used_estimated_bytes bigint NOT NULL DEFAULT 0;
ALTER TABLE nmo_diagnostic_runs ADD COLUMN IF NOT EXISTS profile_ref text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS nmo_diagnostic_requests(actor_id text NOT NULL,request_key text NOT NULL,request_hash bytea NOT NULL,run_id text NOT NULL REFERENCES nmo_diagnostic_runs(id),expires_at timestamptz NOT NULL,PRIMARY KEY(actor_id,request_key));
CREATE INDEX IF NOT EXISTS nmo_diagnostic_runs_history ON nmo_diagnostic_runs(started_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS nmo_diagnostic_runs_monitor ON nmo_diagnostic_runs(monitor,started_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS nmo_alerts(logical_key text PRIMARY KEY,payload jsonb NOT NULL);
ALTER TABLE nmo_alerts ADD COLUMN IF NOT EXISTS id text;
ALTER TABLE nmo_alerts ADD COLUMN IF NOT EXISTS api_view jsonb;
CREATE UNIQUE INDEX IF NOT EXISTS nmo_alerts_id ON nmo_alerts(id);
CREATE TABLE IF NOT EXISTS nmo_alert_acknowledgements(alert_id text NOT NULL REFERENCES nmo_alerts(id),cycle_id text NOT NULL,acknowledged_at timestamptz NOT NULL,actor_id text NOT NULL,actor_name text NOT NULL,note text NOT NULL,PRIMARY KEY(alert_id,cycle_id));
CREATE TABLE IF NOT EXISTS nmo_alert_events(id text PRIMARY KEY,logical_key text NOT NULL,transition jsonb NOT NULL,suppressed boolean NOT NULL,created_at timestamptz NOT NULL DEFAULT now());
CREATE INDEX IF NOT EXISTS nmo_alert_events_history ON nmo_alert_events(logical_key,created_at DESC,id DESC);
CREATE TABLE IF NOT EXISTS nmo_notification_events(id text PRIMARY KEY,kind text NOT NULL,occurred_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS nmo_webhook_outbox(event_id text PRIMARY KEY REFERENCES nmo_alert_events(id),endpoint_hash text NOT NULL,payload jsonb NOT NULL,state text NOT NULL DEFAULT 'pending',attempts integer NOT NULL DEFAULT 0,next_after timestamptz NOT NULL,expires_at timestamptz NOT NULL,lease_token text NOT NULL DEFAULT '',lease_until timestamptz,status_code integer NOT NULL DEFAULT 0);
INSERT INTO nmo_notification_events(id,kind,occurred_at) SELECT o.event_id,COALESCE(o.payload->>'event_type','alert.unknown'),e.created_at FROM nmo_webhook_outbox o JOIN nmo_alert_events e ON e.id=o.event_id ON CONFLICT DO NOTHING;
ALTER TABLE nmo_webhook_outbox DROP CONSTRAINT IF EXISTS nmo_webhook_outbox_event_id_fkey;
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conname='nmo_webhook_notification_event_fkey' AND conrelid='nmo_webhook_outbox'::regclass) THEN ALTER TABLE nmo_webhook_outbox ADD CONSTRAINT nmo_webhook_notification_event_fkey FOREIGN KEY(event_id) REFERENCES nmo_notification_events(id); END IF; END $$;
CREATE TABLE IF NOT EXISTS nmo_webhook_tests(operation_id text PRIMARY KEY REFERENCES nmo_operations(id),event_id text NOT NULL UNIQUE REFERENCES nmo_webhook_outbox(event_id));
CREATE INDEX IF NOT EXISTS nmo_webhook_endpoint_history ON nmo_webhook_outbox(endpoint_hash,event_id);
CREATE INDEX IF NOT EXISTS nmo_webhook_due ON nmo_webhook_outbox(state,next_after);
CREATE TABLE IF NOT EXISTS nmo_webhook_attempts(event_id text NOT NULL REFERENCES nmo_webhook_outbox(event_id),attempt integer NOT NULL,lease_token text NOT NULL,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,status_code integer NOT NULL DEFAULT 0,PRIMARY KEY(event_id,attempt));
CREATE TABLE IF NOT EXISTS nmo_diagnostic_tasks(id text PRIMARY KEY,run_id text NOT NULL REFERENCES nmo_diagnostic_runs(id),agent_id text NOT NULL,monitor text NOT NULL,revision text NOT NULL,zone text NOT NULL,check_id text NOT NULL,deadline timestamptz NOT NULL,check_spec jsonb NOT NULL,state text NOT NULL);
CREATE INDEX IF NOT EXISTS nmo_diagnostic_tasks_run ON nmo_diagnostic_tasks(run_id,id);
CREATE INDEX IF NOT EXISTS nmo_observations_diagnostic_task ON nmo_observations(diagnostic_task_id,id) WHERE diagnostic_task_id<>'';
ALTER TABLE nmo_diagnostic_tasks ADD COLUMN IF NOT EXISTS step_id text NOT NULL DEFAULT '';
ALTER TABLE nmo_diagnostic_tasks ADD COLUMN IF NOT EXISTS estimated_bytes bigint;
ALTER TABLE nmo_diagnostic_tasks ADD COLUMN IF NOT EXISTS attempt_no integer NOT NULL DEFAULT 1;
ALTER TABLE nmo_observations ADD COLUMN IF NOT EXISTS fallback_task_id text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS nmo_fallback_runs(id text PRIMARY KEY,origin_key text UNIQUE NOT NULL,monitor text NOT NULL,revision text NOT NULL,started_at timestamptz NOT NULL,deadline timestamptz NOT NULL,payload jsonb NOT NULL,used integer NOT NULL DEFAULT 0,state text NOT NULL DEFAULT 'pending');
CREATE TABLE IF NOT EXISTS nmo_fallback_tasks(id text PRIMARY KEY,run_id text NOT NULL REFERENCES nmo_fallback_runs(id),agent_id text NOT NULL,zone text NOT NULL,monitor text NOT NULL,revision text NOT NULL,check_id text NOT NULL,deadline timestamptz NOT NULL,attempt_no integer NOT NULL,state text NOT NULL DEFAULT 'reserved',UNIQUE(run_id,attempt_no),UNIQUE(run_id,agent_id));
CREATE TABLE IF NOT EXISTS nmo_config_rejections(agent_id text NOT NULL,revision text NOT NULL,rejected_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(agent_id,revision));
CREATE INDEX IF NOT EXISTS nmo_fallback_history ON nmo_fallback_runs(monitor,id);
CREATE INDEX IF NOT EXISTS nmo_fallback_chronology ON nmo_fallback_runs(monitor,started_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS nmo_fallback_chronology_all ON nmo_fallback_runs(started_at DESC,id DESC);
CREATE INDEX IF NOT EXISTS nmo_fallback_pending_deadline ON nmo_fallback_runs(deadline) WHERE state='pending';
CREATE INDEX IF NOT EXISTS nmo_fallback_observations ON nmo_observations(fallback_task_id) WHERE fallback_task_id<>'';
`)
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
func (s *Store) Assign(ctx context.Context, agent string, generation int64, revision, zone, monitorName string, regular ...bool) error {
	return s.assignChecks(ctx, agent, generation, revision, zone, monitorName, nil, regular...)
}

// AssignPlan records exact historical check ownership for reconnect/replay.
func (s *Store) AssignPlan(ctx context.Context, agent string, generation int64, p monitor.Plan, zone string) error {
	checks := []string{}
	for _, c := range p.Checks {
		if p.CheckAllowed(c.ID, agent) {
			checks = append(checks, c.ID)
		}
	}
	return s.assignChecks(ctx, agent, generation, p.Revision, zone, p.Name, checks, p.RegularAllowed(agent))
}
func (s *Store) assignChecks(ctx context.Context, agent string, generation int64, revision, zone, monitorName string, checks []string, regular ...bool) error {
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
	enabled := true
	if len(regular) > 0 {
		enabled = regular[0]
	}
	if _, e = tx.Exec(ctx, `INSERT INTO nmo_assignments(agent_id,revision,zone,monitor,regular_enabled,allowed_checks) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, agent, revision, zone, monitorName, enabled, checks); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// Applied records each independently acknowledged plan within a fenced stream.
func (s *Store) Applied(ctx context.Context, agent string, generation int64, revision string) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var actual int64
	if e = tx.QueryRow(ctx, `SELECT generation FROM nmo_sessions WHERE agent_id=$1 FOR UPDATE`, agent).Scan(&actual); e != nil {
		return e
	}
	if actual != generation {
		return ErrFenced
	}
	if _, e = tx.Exec(ctx, `INSERT INTO nmo_session_revisions(agent_id,revision,generation) VALUES($1,$2,$3) ON CONFLICT(agent_id,revision) DO UPDATE SET generation=EXCLUDED.generation`, agent, revision, generation); e != nil {
		return e
	}
	// Retain the legacy column for on-disk compatibility, not authorization.
	if _, e = tx.Exec(ctx, `UPDATE nmo_sessions SET applied_revision=$2 WHERE agent_id=$1`, agent, revision); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `DELETE FROM nmo_config_rejections WHERE agent_id=$1 AND revision=$2`, agent, revision); e != nil {
		return e
	}
	return tx.Commit(ctx)
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
	var applied bool
	e = tx.QueryRow(ctx, `SELECT generation,EXISTS(SELECT 1 FROM nmo_session_revisions r WHERE r.agent_id=$1 AND r.generation=nmo_sessions.generation AND r.revision=$2) FROM nmo_sessions WHERE agent_id=$1 FOR UPDATE`, agent, appliedRevision).Scan(&n, &applied)
	if e != nil {
		return nil, e
	}
	if n != generation || !applied {
		return nil, ErrFenced
	}
	duplicates := make([]bool, len(observations))
	for i, o := range observations {
		var assigned bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_assignments WHERE agent_id=$1 AND revision=$2 AND zone=$3 AND monitor=$4 AND ($5 OR (regular_enabled AND (allowed_checks IS NULL OR $6=ANY(allowed_checks)))))`, agent, o.Revision, o.Zone, o.Monitor, o.DiagnosticTaskID != "" || o.FallbackTaskID != "", o.Check).Scan(&assigned); e != nil {
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
		if o.FallbackTaskID != "" {
			if e = validateFallbackObservation(ctx, tx, o); e != nil {
				return nil, e
			}
		}
		b, e := json.Marshal(o)
		if e != nil {
			return nil, e
		}
		hash := sha256.Sum256(b)
		t, e := tx.Exec(ctx, `INSERT INTO nmo_observations(id,agent_id,monitor,revision,check_id,finished_at,payload,payload_hash,diagnostic_task_id,fallback_task_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(id) DO NOTHING`, o.ID, agent, o.Monitor, o.Revision, o.Check, o.FinishedAt, b, hash[:], o.DiagnosticTaskID, o.FallbackTaskID)
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
func (s *Store) LatestRegular(ctx context.Context, p monitor.Plan, now time.Time) ([]monitor.Observation, error) {
	rows, e := s.pool.Query(ctx, `SELECT payload FROM (SELECT DISTINCT ON(agent_id,check_id) payload,agent_id,check_id FROM nmo_observations WHERE monitor=$1 AND revision=$2 AND diagnostic_task_id='' AND fallback_task_id='' AND finished_at >= $3 ORDER BY agent_id,check_id,finished_at DESC,id DESC) x LIMIT 10001`, p.Name, p.Revision, now.Add(-p.MaxAge))
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
