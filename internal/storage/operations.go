package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
)

func scanOperation(row pgx.Row) (resources.Operation, error) {
	var op resources.Operation
	var data []byte
	err := row.Scan(&op.ID, &op.State, &op.StartedAt, &op.FinishedAt, &data)
	if err != nil {
		return op, err
	}
	var payload struct {
		Error  *resources.OperationError `json:"error"`
		Result resources.ApplyResult     `json:"result"`
	}
	if err = json.Unmarshal(data, &payload); err != nil {
		return op, err
	}
	op.Kind = "config.apply"
	op.Error = payload.Error
	op.Result = payload.Result
	return op, nil
}

const operationColumns = "id,state,started_at,finished_at,payload"

func lookupApply(ctx context.Context, tx pgx.Tx, owner, key string, digest []byte) (resources.Operation, bool, error) {
	if key == "" {
		return resources.Operation{}, false, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE nmo_operations SET request_key=NULL WHERE actor_id=$1 AND request_key=$2 AND idem_expires_at<=now()`, owner, key); err != nil {
		return resources.Operation{}, false, err
	}
	var hash []byte
	var id string
	err := tx.QueryRow(ctx, `SELECT id,request_hash FROM nmo_operations WHERE actor_id=$1 AND request_key=$2`, owner, key).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.Operation{}, false, nil
	}
	if err != nil {
		return resources.Operation{}, false, err
	}
	if !bytes.Equal(hash, digest) {
		return resources.Operation{}, false, resources.ErrIdempotency
	}
	op, err := scanOperation(tx.QueryRow(ctx, `SELECT `+operationColumns+` FROM nmo_operations WHERE id=$1`, id))
	return op, err == nil, err
}
func (s *Store) RecoverApply(ctx context.Context, actor auth.AdminCredential, key string, digest []byte) (resources.Operation, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return resources.Operation{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return resources.Operation{}, false, err
	}
	if _, err = checkAdminScope(ctx, tx, actor, "resources:write"); err != nil {
		return resources.Operation{}, false, err
	}
	op, found, err := lookupApply(ctx, tx, actor.UserID, key, digest)
	if err != nil {
		return op, false, err
	}
	return op, found, tx.Commit(ctx)
}
func (s *Store) ApplyResources(ctx context.Context, actor auth.AdminCredential, batch resources.Batch, registry resources.Registry, digest []byte) (resources.Operation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return resources.Operation{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return resources.Operation{}, err
	}
	username, err := checkAdminScope(ctx, tx, actor, "resources:write")
	if err != nil {
		return resources.Operation{}, err
	}
	prior, found, err := lookupApply(ctx, tx, actor.UserID, batch.RequestID, digest)
	if err != nil {
		return prior, err
	}
	if found {
		return prior, tx.Commit(ctx)
	}
	_, compiled, err := resolveCatalogTx(ctx, tx, registry, batch.Manifests)
	if err != nil {
		return resources.Operation{}, err
	}
	explicit := map[string]bool{}
	result := resources.ApplyResult{DesiredCommitted: true, Targets: []resources.ApplyTarget{}}
	for _, m := range batch.Manifests {
		key := m.Kind + "/" + m.Metadata.Name
		entry, valid := compiled[key]
		if !valid || !resources.Supported(m.Kind) {
			return resources.Operation{}, resources.ErrInvalid
		}
		current, e := scanResource(tx.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind=$2 AND name=$1 FOR UPDATE`, m.Metadata.Name, m.Kind))
		exists := e == nil
		if e != nil && !errors.Is(e, pgx.ErrNoRows) {
			return resources.Operation{}, e
		}
		expected, provided := batch.ExpectedVersions[key]
		if exists && !provided {
			return resources.Operation{}, resources.ErrPreconditionRequired
		}
		if exists && expected == nil {
			return resources.Operation{}, resources.ErrConflict
		}
		if expected != nil && (!exists || current.ResourceVersion != *expected) {
			return resources.Operation{}, resources.ErrVersion
		}
		v, e := saveResourceTx(ctx, tx, m, entry.Revision, current, exists, username)
		if e != nil {
			return resources.Operation{}, e
		}
		explicit[key] = true
		result.Targets = append(result.Targets, resources.ApplyTarget{Kind: m.Kind, Name: m.Metadata.Name, UID: v.UID, ResourceVersion: v.ResourceVersion, EffectiveRevision: entry.Revision, Reason: "awaiting_reconciliation"})
	}
	affected, e := refreshDependents(ctx, tx, compiled, explicit)
	if e != nil {
		return resources.Operation{}, e
	}
	result.Targets = append(result.Targets, affected...)
	id, err := eventID()
	if err != nil {
		return resources.Operation{}, err
	}
	payload, err := json.Marshal(struct {
		Error  *resources.OperationError `json:"error"`
		Result resources.ApplyResult     `json:"result"`
	}{Result: result})
	if err != nil {
		return resources.Operation{}, err
	}
	var requestKey any
	if batch.RequestID != "" {
		requestKey = batch.RequestID
	}
	op, err := scanOperation(tx.QueryRow(ctx, `INSERT INTO nmo_operations(id,actor_id,request_key,request_hash,idem_expires_at,state,payload) VALUES($1,$2,$3,$4,now()+interval '24 hours','accepted',$5) RETURNING `+operationColumns, id, actor.UserID, requestKey, digest, payload))
	if err != nil {
		return op, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, username, "config.apply:"+id); err != nil {
		return op, err
	}
	return op, tx.Commit(ctx)
}
func (s *Store) Operation(ctx context.Context, id string) (resources.Operation, bool, error) {
	op, err := scanOperation(s.pool.QueryRow(ctx, `SELECT `+operationColumns+` FROM nmo_operations WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return op, false, nil
	}
	return op, err == nil, err
}
func (s *Store) Operations(ctx context.Context, before, state string, limit int) ([]resources.Operation, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+operationColumns+` FROM nmo_operations WHERE ($1='' OR id<$1) AND ($2='' OR state=$2) ORDER BY id DESC LIMIT $3`, before, state, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Operation{}
	for rows.Next() {
		op, e := scanOperation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, op)
	}
	return out, rows.Err()
}
