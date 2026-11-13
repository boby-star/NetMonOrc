// Package notify joins alert evaluation and durable delivery in the Controller.
package notify

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/storage"
	"github.com/boby-star/NetMonOrc/internal/webhook"
	"time"
)

type Service struct {
	Store    *storage.Store
	Rule     *alerting.Evaluator
	Endpoint webhook.Endpoint
	Sender   *webhook.Sender
}

func (s *Service) Evaluate(ctx context.Context, a monitor.Assessment) (alerting.State, error) {
	return s.Store.EvaluateAlert(ctx, s.Rule, s.Endpoint, a)
}
func (s *Service) Run(ctx context.Context) error {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		c, done := context.WithTimeout(ctx, 5*time.Second)
		job, e := s.Store.ClaimWebhook(c, s.Endpoint)
		done()
		if e != nil {
			return e
		}
		if job == nil {
			continue
		}
		status := 0
		if s.Sender != nil {
			status, _ = s.Sender.Deliver(ctx, job.ID, job.Payload)
		}
		// Delivery cancellation still leaves a durable attempt; a restart can retry
		// the same event ID. No raw receiver response or credential enters the DB.
		persist, finish := context.WithTimeout(context.Background(), 5*time.Second)
		e = s.Store.FinishWebhook(persist, *job, s.Endpoint, status)
		finish()
		if e != nil {
			return e
		}
	}
}
