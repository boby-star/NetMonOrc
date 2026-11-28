package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/responseview"
	"github.com/jackc/pgx/v5"
)

type ResponseEvent = responseview.Event

// Reserve before dispatch. A lost command is never silently retried or extended.
func (s *Store) ReserveResponse(ctx context.Context, p monitor.Plan, a monitor.Assessment, r monitor.ResponseRule, generation int64) (*ResponseEvent, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var revision string
	if err = tx.QueryRow(ctx, `SELECT effective_revision FROM nmo_resources WHERE kind='Monitor' AND name=$1 FOR SHARE`, p.Name).Scan(&revision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFenced
		}
		return nil, err
	}
	if revision != p.Revision || a.Revision != p.Revision || a.Monitor != p.Name || time.Since(a.At) > 5*time.Second || a.At.After(time.Now().Add(time.Second)) {
		return nil, ErrFenced
	}
	var valid bool
	if err = tx.QueryRow(ctx, `SELECT generation=$2 AND EXISTS(SELECT 1 FROM nmo_session_revisions r WHERE r.agent_id=$1 AND r.generation=$2 AND r.revision=$3) FROM nmo_sessions WHERE agent_id=$1 FOR UPDATE`, r.AgentRef, generation, p.Revision).Scan(&valid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFenced
		}
		return nil, err
	}
	if !valid {
		return nil, ErrFenced
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_response_episodes(monitor,revision,rule_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, p.Name, p.Revision, r.ID); err != nil {
		return nil, err
	}
	var fired bool
	if err = tx.QueryRow(ctx, `SELECT fired FROM nmo_response_episodes WHERE monitor=$1 AND revision=$2 AND rule_id=$3 FOR UPDATE`, p.Name, p.Revision, r.ID).Scan(&fired); err != nil {
		return nil, err
	}
	if fired {
		return nil, nil
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	event := &ResponseEvent{ID: hex.EncodeToString(id), Monitor: p.Name, Revision: p.Revision, RuleID: r.ID, Agent: r.AgentRef, Status: "queued", CreatedAt: now, AcceptUntil: now.Add(10 * time.Second), BlockUntil: now.Add(r.Duration)}
	event.Assessment, err = json.Marshal(a)
	if err != nil {
		return nil, err
	}
	event.Rule, err = json.Marshal(r)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO nmo_responses(id,monitor,revision,rule_id,agent,generation,status,created_at,accept_until,block_until,assessment,rule) VALUES($1,$2,$3,$4,$5,$6,'queued',$7,$8,$9,$10,$11)`, event.ID, p.Name, p.Revision, r.ID, r.AgentRef, generation, now, event.AcceptUntil, event.BlockUntil, event.Assessment, event.Rule)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE nmo_response_episodes SET fired=true WHERE monitor=$1 AND revision=$2 AND rule_id=$3`, p.Name, p.Revision, r.ID)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return event, nil
}

func (s *Store) ResetResponses(ctx context.Context, p monitor.Plan) error {
	_, err := s.pool.Exec(ctx, `UPDATE nmo_response_episodes SET fired=false WHERE monitor=$1 AND revision=$2 AND EXISTS(SELECT 1 FROM nmo_resources WHERE kind='Monitor' AND name=$1 AND effective_revision=$2)`, p.Name, p.Revision)
	return err
}

func (s *Store) ResponseReceipt(ctx context.Context, id, agent string, generation int64, state, reason string) error {
	if state != "applied" && state != "rejected" && state != "failed" && state != "unacknowledged" {
		return fmt.Errorf("invalid response status")
	}
	result, err := s.pool.Exec(ctx, `UPDATE nmo_responses x SET status=$4,reason=$5 WHERE id=$1 AND agent=$2 AND generation=$3 AND status='queued' AND accept_until>now() AND EXISTS(SELECT 1 FROM nmo_sessions s WHERE s.agent_id=$2 AND s.generation=$3)`, id, agent, generation, state, reason)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrFenced
	}
	return nil
}

func (s *Store) ResponseHistory(ctx context.Context, name string) ([]ResponseEvent, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,monitor,revision,rule_id,agent,CASE WHEN status='applied' AND block_until<=now() THEN 'expired' WHEN status='queued' AND accept_until<=now() THEN 'unacknowledged' ELSE status END,reason,created_at,accept_until,block_until,assessment,rule FROM nmo_responses WHERE monitor=$1 ORDER BY created_at DESC,id DESC LIMIT 50`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResponseEvent{}
	for rows.Next() {
		var v ResponseEvent
		if err = rows.Scan(&v.ID, &v.Monitor, &v.Revision, &v.RuleID, &v.Agent, &v.Status, &v.Reason, &v.CreatedAt, &v.AcceptUntil, &v.BlockUntil, &v.Assessment, &v.Rule); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
