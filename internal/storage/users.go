package storage

import (
	"context"
	"errors"
	"slices"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const userColumns = "id,username,role,disabled,created_at,version"

func scanUser(row pgx.Row) (auth.UserView, error) {
	var v auth.UserView
	err := row.Scan(&v.ID, &v.Username, &v.Role, &v.Disabled, &v.CreatedAt, &v.Version)
	return v, err
}
func (s *Store) ListUsers(ctx context.Context, before string, limit int) ([]auth.UserView, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userColumns+` FROM nmo_users WHERE ($1='' OR id<$1) ORDER BY id DESC LIMIT $2`, before, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []auth.UserView{}
	for rows.Next() {
		v, e := scanUser(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) GetUser(ctx context.Context, id string) (auth.UserView, bool, error) {
	v, err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM nmo_users WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, err
}
func checkAdmin(ctx context.Context, tx pgx.Tx, actor auth.AdminCredential) (string, error) {
	if actor.UserID == "" || len(actor.Hash) != 32 {
		return "", auth.ErrCredentials
	}
	var username, role string
	var disabled bool
	err := tx.QueryRow(ctx, `SELECT username,role,disabled FROM nmo_users WHERE id=$1 FOR UPDATE`, actor.UserID).Scan(&username, &role, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrCredentials
	}
	if err != nil {
		return "", err
	}
	if disabled {
		return "", auth.ErrCredentials
	}
	if role != "admin" {
		return "", auth.ErrForbidden
	}
	if actor.PAT {
		var scopes []string
		err = tx.QueryRow(ctx, `SELECT scopes FROM nmo_api_tokens WHERE user_id=$1 AND token_hash=$2 AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at>now()) FOR SHARE`, actor.UserID, actor.Hash).Scan(&scopes)
		if err == nil && !slices.Contains(scopes, "system:admin") {
			return "", auth.ErrForbidden
		}
	} else {
		var hash []byte
		err = tx.QueryRow(ctx, `SELECT token_hash FROM nmo_browser_sessions WHERE user_id=$1 AND token_hash=$2 AND expires_at>now() FOR SHARE`, actor.UserID, actor.Hash).Scan(&hash)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", auth.ErrCredentials
	}
	return username, err
}
func (s *Store) CreateUser(ctx context.Context, actor auth.AdminCredential, in auth.UserCreate, hash string) (auth.UserView, error) {
	var empty auth.UserView
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	// All user-role mutations share this transaction lock, including across processes.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781304)`); err != nil {
		return empty, err
	}
	username, err := checkAdmin(ctx, tx, actor)
	if err != nil {
		return empty, err
	}
	id, err := eventID()
	if err != nil {
		return empty, err
	}
	v, err := scanUser(tx.QueryRow(ctx, `INSERT INTO nmo_users(id,username,password_hash,role) VALUES($1,$2,$3,$4) RETURNING `+userColumns, id, in.Username, hash, in.Role))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return empty, auth.ErrConflict
		}
		return empty, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, username, "user.create:"+id); err != nil {
		return empty, err
	}
	return v, tx.Commit(ctx)
}
func (s *Store) PatchUser(ctx context.Context, actor auth.AdminCredential, id string, expected int64, patch auth.UserPatch) (auth.UserView, error) {
	var empty auth.UserView
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781304)`); err != nil {
		return empty, err
	}
	username, err := checkAdmin(ctx, tx, actor)
	if err != nil {
		return empty, err
	}
	current, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM nmo_users WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, auth.ErrNotFound
	}
	if err != nil {
		return empty, err
	}
	if current.Version != expected {
		return empty, auth.ErrPrecondition
	}
	role, disabled := current.Role, current.Disabled
	if patch.Role != nil {
		role = *patch.Role
	}
	if patch.Disabled != nil {
		disabled = *patch.Disabled
	}
	if current.Role == "admin" && !current.Disabled && (role != "admin" || disabled) {
		var active int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM nmo_users WHERE role='admin' AND NOT disabled`).Scan(&active); err != nil {
			return empty, err
		}
		if active <= 1 {
			return empty, auth.ErrConflict
		}
	}
	v, err := scanUser(tx.QueryRow(ctx, `UPDATE nmo_users SET role=$2,disabled=$3,version=version+1 WHERE id=$1 RETURNING `+userColumns, id, role, disabled))
	if err != nil {
		return empty, err
	}
	if role != current.Role || (disabled && !current.Disabled) {
		// Re-enabling or promoting later must not resurrect a previously issued credential.
		if _, err = tx.Exec(ctx, `DELETE FROM nmo_browser_sessions WHERE user_id=$1`, id); err != nil {
			return empty, err
		}
		if _, err = tx.Exec(ctx, `UPDATE nmo_api_tokens SET revoked_at=now() WHERE user_id=$1 AND revoked_at IS NULL`, id); err != nil {
			return empty, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, username, "user.patch:"+id); err != nil {
		return empty, err
	}
	return v, tx.Commit(ctx)
}
