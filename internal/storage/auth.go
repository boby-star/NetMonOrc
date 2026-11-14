package storage

import (
	"context"
	"errors"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/jackc/pgx/v5"
)

func (s *Store) BootstrapUser(ctx context.Context, username, hash string) (bool, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(781303)"); e != nil {
		return false, e
	}
	var exists bool
	if e = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM nmo_users)").Scan(&exists); e != nil {
		return false, e
	}
	if exists {
		return false, tx.Commit(ctx)
	}
	id, e := eventID()
	if e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO nmo_users(id,username,password_hash,role) VALUES($1,$2,$3,'admin')", id, username, hash); e != nil {
		return false, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,'bootstrap',true)", username); e != nil {
		return false, e
	}
	return true, tx.Commit(ctx)
}
func (s *Store) FindUser(ctx context.Context, name string) (auth.User, bool, error) {
	var u auth.User
	e := s.pool.QueryRow(ctx, "SELECT id,username,role,password_hash,disabled FROM nmo_users WHERE username=$1", name).Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.Disabled)
	if errors.Is(e, pgx.ErrNoRows) {
		return u, false, nil
	}
	return u, e == nil, e
}
func (s *Store) FinishLogin(ctx context.Context, expected auth.User, correct bool, hash []byte, expiry time.Time) (bool, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	success := false
	if expected.ID != "" {
		var current, role string
		var disabled bool
		var failures int
		var locked *time.Time
		e = tx.QueryRow(ctx, "SELECT password_hash,role,disabled,failed_logins,locked_until FROM nmo_users WHERE id=$1 FOR UPDATE", expected.ID).Scan(&current, &role, &disabled, &failures, &locked)
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return false, e
		}
		if e == nil {
			now := time.Now()
			isLocked := locked != nil && now.Before(*locked)
			if locked != nil && !isLocked {
				failures = 0
				locked = nil
			}
			success = correct && role == expected.Role && current == expected.PasswordHash && !disabled && !isLocked
			if success {
				failures = 0
				locked = nil
			} else if !isLocked {
				failures++
				if failures >= 5 {
					until := now.Add(15 * time.Minute)
					locked = &until
				}
			}
			if _, e = tx.Exec(ctx, "UPDATE nmo_users SET failed_logins=$2,locked_until=$3 WHERE id=$1", expected.ID, failures, locked); e != nil {
				return false, e
			}
		}
	}
	if _, e = tx.Exec(ctx, "INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,'login', $2)", expected.Username, success); e != nil {
		return false, e
	}
	if success {
		if _, e = tx.Exec(ctx, "DELETE FROM nmo_browser_sessions WHERE expires_at<=now()"); e != nil {
			return false, e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO nmo_browser_sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)", hash, expected.ID, expiry); e != nil {
			return false, e
		}
	}
	return success, tx.Commit(ctx)
}
func (s *Store) SessionUser(ctx context.Context, hash []byte) (auth.User, time.Time, bool, error) {
	var u auth.User
	var expiry time.Time
	e := s.pool.QueryRow(ctx, "SELECT u.id,u.username,u.role,u.disabled,s.expires_at FROM nmo_browser_sessions s JOIN nmo_users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT u.disabled", hash).Scan(&u.ID, &u.Username, &u.Role, &u.Disabled, &expiry)
	if errors.Is(e, pgx.ErrNoRows) {
		return u, expiry, false, nil
	}
	return u, expiry, e == nil, e
}
func (s *Store) RevokeSession(ctx context.Context, hash []byte) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, "INSERT INTO nmo_identity_audit(username,action,succeeded) SELECT u.username,'logout',true FROM nmo_users u JOIN nmo_browser_sessions s ON s.user_id=u.id WHERE s.token_hash=$1", hash)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "DELETE FROM nmo_browser_sessions WHERE token_hash=$1", hash); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

func (s *Store) HasUsers(ctx context.Context) (bool, error) {
	var found bool
	err := s.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM nmo_users)").Scan(&found)
	return found, err
}
