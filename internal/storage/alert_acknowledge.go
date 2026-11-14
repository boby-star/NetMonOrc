package storage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/alertview"
	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
)

func (s *Store) AcknowledgeAlert(ctx context.Context, actor auth.AdminCredential, id, expected string, request alertview.Acknowledge) (alertview.Alert, error) {
	var v alertview.Alert
	if !utf8.ValidString(request.Note) || utf8.RuneCountInString(request.Note) > 1024 || strings.ContainsRune(request.Note, 0) {
		return v, resources.ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return v, err
	}
	defer tx.Rollback(ctx)
	username, err := checkAdminScope(ctx, tx, actor, "alerts:manage")
	if err != nil {
		return v, err
	}
	var raw, stateJSON []byte
	err = tx.QueryRow(ctx, `SELECT api_view,payload FROM nmo_alerts WHERE id=$1 AND api_view IS NOT NULL FOR UPDATE`, id).Scan(&raw, &stateJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, resources.ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(raw, &v); err != nil {
		return v, err
	}
	if expected != alertview.Version(v) {
		return v, resources.ErrVersion
	}
	var state alerting.State
	if err = json.Unmarshal(stateJSON, &state); err != nil {
		return v, err
	}
	if v.State != "firing" || state.Status != v.State || (v.FiredAt != nil && !v.FiredAt.Equal(state.LastTransitionAt)) {
		return v, resources.ErrConflict
	}
	cycle := alertview.Cycle(v)
	if v.AcknowledgedAt != nil {
		var note string
		if err = tx.QueryRow(ctx, `SELECT note FROM nmo_alert_acknowledgements WHERE alert_id=$1 AND cycle_id=$2`, id, cycle).Scan(&note); errors.Is(err, pgx.ErrNoRows) {
			return v, resources.ErrConflict
		} else if err != nil {
			return v, err
		}
		if note != request.Note {
			return v, resources.ErrConflict
		}
		return v, tx.Commit(ctx)
	}
	at := time.Now().UTC()
	v.AcknowledgedAt = &at
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_alert_acknowledgements(alert_id,cycle_id,acknowledged_at,actor_id,actor_name,note) VALUES($1,$2,$3,$4,$5,$6)`, id, cycle, at, actor.UserID, username, request.Note); err != nil {
		return v, err
	}
	raw, err = json.Marshal(v)
	if err != nil {
		return v, err
	}
	// The FSM payload, transition history and webhook outbox are deliberately not written.
	if _, err = tx.Exec(ctx, `UPDATE nmo_alerts SET api_view=$2 WHERE id=$1`, id, raw); err != nil {
		return v, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, username, "alert.acknowledge:"+id); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
