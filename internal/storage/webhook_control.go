package storage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/operationview"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/boby-star/NetMonOrc/internal/webhook"
	"github.com/jackc/pgx/v5"
)

// queueNotification owns queue event identities independently of Alert lifecycle.
func queueNotification(ctx context.Context, tx pgx.Tx, id, kind string, at time.Time, endpoint webhook.Endpoint, payload []byte) error {
	if _, err := tx.Exec(ctx, `INSERT INTO nmo_notification_events(id,kind,occurred_at) VALUES($1,$2,$3)`, id, kind, at); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO nmo_webhook_outbox(event_id,endpoint_hash,payload,next_after,expires_at) VALUES($1,$2,$3,$4,$5)`, id, endpoint.Hash(), payload, at, at.Add(24*time.Hour))
	return err
}
func (s *Store) QueueWebhookTest(ctx context.Context, actor auth.AdminCredential, endpoint webhook.Endpoint) (operationview.Operation, error) {
	var empty operationview.Operation
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback(ctx)
	if current, err := s.webhookCurrent(ctx, tx, endpoint); err != nil {
		return empty, err
	} else if !current {
		return empty, resources.ErrConflict
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781306)`); err != nil {
		return empty, err
	}
	username, err := checkAdminScope(ctx, tx, actor, "alerts:manage")
	if err != nil {
		return empty, err
	}
	if !endpoint.Enabled || endpoint.Name == "" {
		return empty, resources.ErrConflict
	}
	var busy bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_webhook_tests t JOIN nmo_webhook_outbox o ON o.event_id=t.event_id WHERE o.endpoint_hash=$1 AND o.state IN ('pending','leased') AND o.expires_at>now())`, endpoint.Hash()).Scan(&busy); err != nil {
		return empty, err
	}
	if busy {
		return empty, resources.ErrConflict
	}
	id, err := eventID()
	if err != nil {
		return empty, err
	}
	eventID, err := eventID()
	if err != nil {
		return empty, err
	}
	now := time.Now().UTC()
	event := webhook.TestEvent{ID: eventID, Type: "webhook.test", At: now, Schema: "1"}
	body, err := json.Marshal(event)
	if err != nil {
		return empty, err
	}
	if err = queueNotification(ctx, tx, eventID, event.Type, now, endpoint, body); err != nil {
		return empty, err
	}
	result := webhook.TestResult{EventID: eventID, DeliveryState: "pending"}
	payload, err := json.Marshal(struct {
		Error  *operationview.Error `json:"error"`
		Result webhook.TestResult   `json:"result"`
	}{nil, result})
	if err != nil {
		return empty, err
	}
	hash := sha256.Sum256([]byte("{}"))
	op, err := scanOperationView(tx.QueryRow(ctx, `INSERT INTO nmo_operations(id,actor_id,request_hash,idem_expires_at,state,kind,payload) VALUES($1,$2,$3,$4,'accepted','webhook.test',$5) RETURNING id,kind,state,started_at,finished_at,payload`, id, actor.UserID, hash[:], now.Add(24*time.Hour), payload))
	if err != nil {
		return empty, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_webhook_tests(operation_id,event_id) VALUES($1,$2)`, id, eventID); err != nil {
		return empty, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, username, "webhook.test:"+id); err != nil {
		return empty, err
	}
	return op, tx.Commit(ctx)
}

// Called in the same transaction as queue/attempt state changes. No poll creates work.
func refreshWebhookOperations(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `UPDATE nmo_operations p SET
 state=CASE WHEN o.state='delivered' THEN 'completed' WHEN o.state IN ('failed','cancelled') THEN 'failed' WHEN o.state='leased' OR o.attempts>0 THEN 'running' ELSE 'accepted' END,
 finished_at=CASE WHEN o.state IN ('delivered','failed','cancelled') THEN COALESCE(p.finished_at,now()) ELSE NULL END,
 updated_at=now(),payload=jsonb_build_object(
 'error',CASE WHEN o.state='failed' THEN jsonb_build_object('code','WEBHOOK_DELIVERY_FAILED','message','Webhook delivery exhausted its attempts or TTL') WHEN o.state='cancelled' THEN jsonb_build_object('code','WEBHOOK_ENDPOINT_CHANGED','message','Webhook endpoint revision changed') ELSE NULL END,
 'result',jsonb_build_object('eventId',o.event_id,'deliveryState',o.state,'attempts',o.attempts,'lastHttpStatus',NULLIF(o.status_code,0)))
 FROM nmo_webhook_tests t JOIN nmo_webhook_outbox o ON o.event_id=t.event_id WHERE p.id=t.operation_id AND p.kind='webhook.test' AND p.state IN ('accepted','running')`)
	return err
}
