package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
)

var ErrDiagnosticTerminal = errors.New("diagnostic run already terminal")

type manualAdmission struct {
	Actor   auth.AdminCredential
	Request diagnosticview.Start
}

// Idempotency recovery also rechecks the current credential inside the transaction.
func (s *Store) RecoverDiagnosticStart(ctx context.Context, actor auth.AdminCredential, request diagnosticview.Start) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err = checkAdminScope(ctx, tx, actor, "diagnostics:run"); err != nil {
		return "", err
	}
	id, err := recoverDiagnosticStart(ctx, tx, actor.UserID, request)
	return id, err
}
func recoverDiagnosticStart(ctx context.Context, tx pgx.Tx, owner string, request diagnosticview.Start) (string, error) {
	if request.RequestID == "" {
		return "", nil
	}
	var id string
	var saved []byte
	hash := diagnosticRequestHash(request)
	err := tx.QueryRow(ctx, `SELECT run_id,request_hash FROM nmo_diagnostic_requests WHERE actor_id=$1 AND request_key=$2 AND expires_at>now()`, owner, request.RequestID).Scan(&id, &saved)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !bytes.Equal(saved, hash[:]) {
		return "", resources.ErrIdempotency
	}
	return id, nil
}
func (s *Store) StartManualDiagnostic(ctx context.Context, p monitor.Plan, profile diagnostics.Profile, actor auth.AdminCredential, request diagnosticview.Start) (string, error) {
	return s.admitDiagnostic(ctx, p, profile, &manualAdmission{Actor: actor, Request: request})
}
func (s *Store) admitDiagnostic(ctx context.Context, p monitor.Plan, profile diagnostics.Profile, manual *manualAdmission) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return "", err
	}
	actor := ""
	if manual != nil {
		actor, err = checkAdminScope(ctx, tx, manual.Actor, "diagnostics:run")
		if err != nil {
			return "", err
		}
		id, e := recoverDiagnosticStart(ctx, tx, manual.Actor.UserID, manual.Request)
		if e != nil {
			return "", e
		}
		if id != "" {
			return id, tx.Commit(ctx)
		}
	}
	// A former runtime must not launch new work during a desired revision rollout.
	var desired string
	err = tx.QueryRow(ctx, `SELECT effective_revision FROM nmo_resources WHERE kind='Monitor' AND name=$1`, p.Name).Scan(&desired)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if (err == nil && desired != p.Revision) || (manual != nil && errors.Is(err, pgx.ErrNoRows)) {
		return "", resources.ErrConflict
	}
	if p.Diagnostics == nil || !p.Diagnostics.Enabled {
		return "", resources.ErrConflict
	}
	healthy := false
	if manual != nil {
		var hash string
		if err = tx.QueryRow(ctx, `SELECT effective_revision FROM nmo_resources WHERE kind='DiagnosticProfile' AND name=$1`, profile.Name).Scan(&hash); errors.Is(err, pgx.ErrNoRows) {
			return "", resources.ErrNotFound
		} else if err != nil {
			return "", err
		}
		if hash != p.Diagnostics.ProfileHash {
			return "", resources.ErrConflict
		}
		var raw []byte
		var at time.Time
		var revision string
		if err = tx.QueryRow(ctx, `SELECT payload,evaluated_at,revision FROM nmo_assessments WHERE monitor=$1`, p.Name).Scan(&raw, &at, &revision); errors.Is(err, pgx.ErrNoRows) {
			return "", resources.ErrConflict
		} else if err != nil {
			return "", err
		}
		age := time.Since(at)
		if revision != p.Revision || age > 5*time.Second || age < -5*time.Second {
			return "", resources.ErrConflict
		}
		var a monitor.Assessment
		if err = json.Unmarshal(raw, &a); err != nil {
			return "", err
		}
		healthy = a.State == "healthy" && len(a.Zones) > 0
		for _, zone := range a.Zones {
			healthy = healthy && zone.Coverage == "complete"
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_incidents(monitor) VALUES($1) ON CONFLICT DO NOTHING`, p.Name); err != nil {
		return "", err
	}
	var episode, run string
	var last *time.Time
	if err = tx.QueryRow(ctx, `SELECT episode_id,run_id,last_finished FROM nmo_incidents WHERE monitor=$1 FOR UPDATE`, p.Name).Scan(&episode, &run, &last); err != nil {
		return "", err
	}
	if healthy && run != "" {
		var state string
		if err = tx.QueryRow(ctx, `SELECT state FROM nmo_diagnostic_runs WHERE id=$1`, run).Scan(&state); err != nil {
			return "", err
		}
		if state != "running" {
			episode = ""
			run = ""
		}
	}
	if run != "" || (last != nil && time.Since(*last) < p.Diagnostics.Cooldown) {
		return "", nil
	}
	if episode == "" {
		episode = newID()
	}
	run = newID()
	if episode == "" || run == "" {
		return "", errors.New("diagnostic identity unavailable")
	}
	mode := profile.Mode
	if p.Diagnostics.ComparisonStatic {
		mode = "static"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_diagnostic_runs(id,episode_id,monitor,revision,profile_hash,mode,max_checks,deadline,profile_ref,state) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'running')`, run, episode, p.Name, p.Revision, p.Diagnostics.ProfileHash, mode, profile.Budget.MaxChecks, time.Now().Add(profile.Budget.MaxDuration), profile.Name); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `UPDATE nmo_incidents SET episode_id=$2,run_id=$3 WHERE monitor=$1`, p.Name, episode, run); err != nil {
		return "", err
	}
	if manual != nil {
		if manual.Request.RequestID != "" {
			hash := diagnosticRequestHash(manual.Request)
			if _, err = tx.Exec(ctx, `INSERT INTO nmo_diagnostic_requests(actor_id,request_key,request_hash,run_id,expires_at) VALUES($1,$2,$3,$4,now()+interval '24 hours') ON CONFLICT(actor_id,request_key) DO UPDATE SET request_hash=excluded.request_hash,run_id=excluded.run_id,expires_at=excluded.expires_at`, manual.Actor.UserID, manual.Request.RequestID, hash[:], run); err != nil {
				return "", err
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, actor, "diagnostic.start:"+run); err != nil {
			return "", err
		}
	}
	return run, tx.Commit(ctx)
}
func (s *Store) CancelDiagnostic(ctx context.Context, actor auth.AdminCredential, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return err
	}
	username, err := checkAdminScope(ctx, tx, actor, "diagnostics:run")
	if err != nil {
		return err
	}
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM nmo_diagnostic_runs WHERE id=$1 FOR UPDATE`, id).Scan(&state); errors.Is(err, pgx.ErrNoRows) {
		return resources.ErrNotFound
	} else if err != nil {
		return err
	}
	if state == "cancelled" {
		return tx.Commit(ctx)
	}
	if state != "running" {
		return resources.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE nmo_diagnostic_runs SET state='cancelled',reason='operator_cancelled',report=NULL,finished_at=now() WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE nmo_diagnostic_tasks SET state='cancelled' WHERE run_id=$1 AND state IN ('reserved','accepted')`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE nmo_incidents SET last_finished=now() WHERE run_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, username, "diagnostic.cancel:"+id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func diagnosticRequestHash(r diagnosticview.Start) [32]byte {
	b, _ := json.Marshal(struct {
		Monitor string
		Request diagnosticview.Start
	}{r.Monitor, r})
	return sha256.Sum256(b)
}
