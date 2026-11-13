package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/webhook"
	"sort"
	"time"
)

func eventID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:], nil
}
func (s *Store) EvaluateAlert(ctx context.Context, rule *alerting.Evaluator, endpoint webhook.Endpoint, a monitor.Assessment) (alerting.State, error) {
	if rule.Rule.Monitor != a.Monitor || rule.Rule.Channel != endpoint.Name {
		return alerting.State{}, fmt.Errorf("alert monitor/channel mismatch")
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return alerting.State{}, e
	}
	defer tx.Rollback(ctx)
	key := rule.Rule.Name + "/" + a.Monitor + "/monitor"
	_, e = tx.Exec(ctx, `INSERT INTO nmo_alerts(logical_key,payload) VALUES($1,'{}') ON CONFLICT DO NOTHING`, key)
	if e != nil {
		return alerting.State{}, e
	}
	var b []byte
	if e = tx.QueryRow(ctx, `SELECT payload FROM nmo_alerts WHERE logical_key=$1 FOR UPDATE`, key).Scan(&b); e != nil {
		return alerting.State{}, e
	}
	var old alerting.State
	if e = json.Unmarshal(b, &old); e != nil {
		return old, e
	}
	next := rule.Step(ctx, old, a)
	if next.Transition != nil {
		id, err := eventID()
		if err != nil {
			return old, err
		}
		transition, err := json.Marshal(next.Transition)
		if err != nil {
			return old, err
		}
		_, e = tx.Exec(ctx, `INSERT INTO nmo_alert_events(id,logical_key,transition,suppressed) VALUES($1,$2,$3,$4)`, id, key, transition, next.Suppressed)
		if e != nil {
			return old, e
		}
		if next.Notify != "" && !next.Suppressed && endpoint.Enabled {
			reasons, evidence := map[string]bool{}, map[string]bool{}
			for _, z := range a.Zones {
				reasons[z.Reason] = true
				for _, id := range z.Evidence {
					evidence[id] = true
				}
			}
			event := alerting.Event{ID: id, Type: "alert." + next.Notify, Rule: rule.Rule.Name, Monitor: a.Monitor, State: a.State, Severity: rule.Rule.Severity, Reasons: []string{}, Evidence: []string{}, At: a.At, Schema: "1"}
			for r := range reasons {
				event.Reasons = append(event.Reasons, r)
			}
			for id := range evidence {
				event.Evidence = append(event.Evidence, id)
			}
			sort.Strings(event.Reasons)
			sort.Strings(event.Evidence)
			body, err := json.Marshal(event)
			if err != nil {
				return old, err
			}
			_, e = tx.Exec(ctx, `INSERT INTO nmo_webhook_outbox(event_id,endpoint_hash,payload,next_after,expires_at) VALUES($1,$2,$3,$4,$5)`, id, endpoint.Hash(), body, a.At, a.At.Add(24*time.Hour))
			if e != nil {
				return old, e
			}
			t := a.At
			if next.Notify == "firing" {
				next.State.LastFiringQueuedAt = &t
			} else {
				next.State.LastResolvedQueuedAt = &t
			}
		}
	}
	b, e = json.Marshal(next.State)
	if e != nil {
		return old, e
	}
	if _, e = tx.Exec(ctx, `UPDATE nmo_alerts SET payload=$2 WHERE logical_key=$1`, key, b); e != nil {
		return old, e
	}
	return next.State, tx.Commit(ctx)
}

type Delivery struct {
	ID, Lease string
	Payload   []byte
	Attempt   int
}

func (s *Store) ClaimWebhook(ctx context.Context, endpoint webhook.Endpoint) (*Delivery, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	// Cancellation on endpoint changes prevents disclosing an older event to a
	// replacement destination. Terminal rows remain available for review.
	_, e = tx.Exec(ctx, `UPDATE nmo_webhook_outbox SET state=CASE WHEN endpoint_hash<>$1 THEN 'cancelled' ELSE 'failed' END,lease_token='' WHERE state IN ('pending','leased') AND (endpoint_hash<>$1 OR expires_at<=now() OR (attempts>=$2 AND (state='pending' OR lease_until<=now())))`, endpoint.Hash(), endpoint.MaxAttempts)
	if e != nil {
		return nil, e
	}
	if !endpoint.Enabled {
		return nil, tx.Commit(ctx)
	}
	var job Delivery
	rows, e := tx.Query(ctx, `SELECT event_id,payload,attempts FROM nmo_webhook_outbox WHERE (state='pending' AND next_after<=now()) OR (state='leased' AND lease_until<=now()) ORDER BY next_after,event_id FOR UPDATE SKIP LOCKED LIMIT 1`)
	if e != nil {
		return nil, e
	}
	if !rows.Next() {
		err := rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		return nil, tx.Commit(ctx)
	}
	if e = rows.Scan(&job.ID, &job.Payload, &job.Attempt); e != nil {
		rows.Close()
		return nil, e
	}
	rows.Close()
	job.Lease, e = eventID()
	if e != nil {
		return nil, e
	}
	job.Attempt++
	_, e = tx.Exec(ctx, `UPDATE nmo_webhook_outbox SET state='leased',attempts=$2,lease_token=$3,lease_until=$4 WHERE event_id=$1`, job.ID, job.Attempt, job.Lease, time.Now().Add(endpoint.Timeout+5*time.Second))
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO nmo_webhook_attempts(event_id,attempt,lease_token) VALUES($1,$2,$3)`, job.ID, job.Attempt, job.Lease)
	if e != nil {
		return nil, e
	}
	return &job, tx.Commit(ctx)
}
func (s *Store) FinishWebhook(ctx context.Context, job Delivery, endpoint webhook.Endpoint, status int) error {
	state := "pending"
	if status >= 200 && status < 300 {
		state = "delivered"
	} else if job.Attempt >= endpoint.MaxAttempts {
		state = "failed"
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, `UPDATE nmo_webhook_outbox SET state=$3,status_code=$4,next_after=$5,lease_token='' WHERE event_id=$1 AND lease_token=$2 AND state='leased'`, job.ID, job.Lease, state, status, time.Now().Add(endpoint.Backoff(job.Attempt)))
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("webhook lease superseded")
	}
	_, e = tx.Exec(ctx, `UPDATE nmo_webhook_attempts SET finished_at=now(),status_code=$3 WHERE event_id=$1 AND lease_token=$2`, job.ID, job.Lease, status)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
