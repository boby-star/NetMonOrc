package storage

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
)

// ReconcileOperations rotates a bounded number of pending operations. Readiness
// requires current stream proof AND its persisted, fenced session generation.
func (s *Store) ReconcileOperations(ctx context.Context, active, revision string, slots []monitor.Slot, proofs []monitor.AppliedAssignment) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT `+operationColumns+` FROM nmo_operations WHERE state IN ('accepted','running') ORDER BY updated_at ASC,id ASC LIMIT 10 FOR UPDATE`)
	if err != nil {
		return err
	}
	pending := []resources.Operation{}
	for rows.Next() {
		op, e := scanOperation(rows)
		if e != nil {
			rows.Close()
			return e
		}
		pending = append(pending, op)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return tx.Commit(ctx)
	}
	ready := map[string]monitor.AppliedAssignment{}
	for _, proof := range proofs {
		if proof.Revision != revision {
			continue
		}
		var generation int64
		var applied string
		err = tx.QueryRow(ctx, `SELECT generation,applied_revision FROM nmo_sessions WHERE agent_id=$1 FOR SHARE`, proof.Agent).Scan(&generation, &applied)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if generation == proof.Generation && applied == revision {
			ready[proof.Agent] = proof
		}
	}
	allAgentsReady := len(slots) > 0
	for _, slot := range slots {
		proof, ok := ready[slot.Agent]
		if !ok || proof.Zone != slot.Zone {
			allAgentsReady = false
		}
	}
	for _, op := range pending {
		allReady := len(op.Result.Targets) > 0
		loaded := false
		for i := range op.Result.Targets {
			target := &op.Result.Targets[i]
			target.Ready = false
			target.Reason = "controller_not_selected"
			current, e := scanResource(tx.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind='Monitor' AND name=$1`, target.Name))
			if e != nil && !errors.Is(e, pgx.ErrNoRows) {
				return e
			}
			if errors.Is(e, pgx.ErrNoRows) || current.UID != target.UID || current.ResourceVersion != target.ResourceVersion || current.Revision != target.EffectiveRevision {
				op.State = "failed"
				target.Reason = "superseded"
				op.Error = &resources.OperationError{Code: "CONFIG_SUPERSEDED", Message: "A target was removed or replaced before reconciliation completed"}
				allReady = false
				break
			}
			if target.Name == active {
				target.Reason = "awaiting_controller_revision"
				if target.EffectiveRevision == revision {
					loaded = true
					target.Reason = "awaiting_agent_acknowledgements"
					if allAgentsReady {
						target.Ready = true
						target.Reason = "required_agents_applied"
					}
				}
			}
			allReady = allReady && target.Ready
		}
		if op.State != "failed" {
			op.State = "accepted"
			if loaded {
				op.State = "running"
			}
			if allReady {
				op.State = "completed"
			}
		}
		payload, e := json.Marshal(struct {
			Error  *resources.OperationError `json:"error"`
			Result resources.ApplyResult     `json:"result"`
		}{op.Error, op.Result})
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, `UPDATE nmo_operations SET state=$2,payload=$3,updated_at=now(),finished_at=CASE WHEN $2 IN ('completed','failed') THEN now() ELSE NULL END WHERE id=$1`, op.ID, op.State, payload); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
