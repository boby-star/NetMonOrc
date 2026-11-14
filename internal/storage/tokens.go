package storage

import (
	"context"
	"errors"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/jackc/pgx/v5"
)

func (s *Store) IssueAPIToken(ctx context.Context, owner string, session []byte, in auth.APITokenCreate, hash []byte) (auth.APITokenView, error) {
	var view auth.APITokenView
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return view, err
	}
	defer tx.Rollback(ctx)
	var role, username string
	var disabled bool
	err = tx.QueryRow(ctx, `SELECT role,username,disabled FROM nmo_users WHERE id=$1 FOR UPDATE`, owner).Scan(&role, &username, &disabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return view, auth.ErrCredentials
	}
	if err != nil {
		return view, err
	}
	var active []byte
	err = tx.QueryRow(ctx, `SELECT token_hash FROM nmo_browser_sessions WHERE token_hash=$1 AND user_id=$2 AND expires_at>now() FOR SHARE`, session, owner).Scan(&active)
	if errors.Is(err, pgx.ErrNoRows) || disabled {
		return view, auth.ErrCredentials
	}
	if err != nil {
		return view, err
	}
	if !auth.AllowedScopes(role, in.Scopes) {
		return view, auth.ErrForbidden
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now()) {
		return view, auth.ErrInput
	}
	id, err := eventID()
	if err != nil {
		return view, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO nmo_api_tokens(id,user_id,name,token_hash,scopes,expires_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,name,scopes,expires_at,created_at,revoked_at`, id, owner, in.Name, hash, in.Scopes, in.ExpiresAt).Scan(&view.ID, &view.Name, &view.Scopes, &view.ExpiresAt, &view.CreatedAt, &view.RevokedAt)
	if err != nil {
		return view, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,'token.issue',true)`, username); err != nil {
		return view, err
	}
	return view, tx.Commit(ctx)
}
func (s *Store) ListAPITokens(ctx context.Context, owner, before string, limit int) ([]auth.APITokenView, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,name,scopes,expires_at,created_at,revoked_at FROM nmo_api_tokens WHERE user_id=$1 AND ($2='' OR id<$2) ORDER BY id DESC LIMIT $3`, owner, before, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []auth.APITokenView{}
	for rows.Next() {
		var v auth.APITokenView
		if err = rows.Scan(&v.ID, &v.Name, &v.Scopes, &v.ExpiresAt, &v.CreatedAt, &v.RevokedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) RevokeAPIToken(ctx context.Context, owner, id string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	// Only the owning user can address a token. Repeated revocation is idempotent.
	var revoked *time.Time
	err = tx.QueryRow(ctx, `SELECT revoked_at FROM nmo_api_tokens WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, owner).Scan(&revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if revoked == nil {
		if _, err = tx.Exec(ctx, `UPDATE nmo_api_tokens SET revoked_at=now() WHERE id=$1`, id); err != nil {
			return false, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) SELECT username,'token.revoke',true FROM nmo_users WHERE id=$1`, owner); err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}
func (s *Store) APITokenUser(ctx context.Context, hash []byte) (auth.User, []string, *time.Time, bool, error) {
	var u auth.User
	var scopes []string
	var expiry *time.Time
	err := s.pool.QueryRow(ctx, `SELECT u.id,u.username,u.role,u.disabled,t.scopes,t.expires_at FROM nmo_api_tokens t JOIN nmo_users u ON u.id=t.user_id WHERE t.token_hash=$1 AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at>now()) AND NOT u.disabled`, hash).Scan(&u.ID, &u.Username, &u.Role, &u.Disabled, &scopes, &expiry)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, nil, nil, false, nil
	}
	return u, scopes, expiry, err == nil, err
}
