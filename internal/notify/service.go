// Package notify joins alert evaluation and durable delivery in the Controller.
package notify

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/boby-star/NetMonOrc/internal/alerting"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/boby-star/NetMonOrc/internal/storage"
	"github.com/boby-star/NetMonOrc/internal/webhook"
	"time"
)

type Service struct {
	Store    *storage.Store
	Managed  bool
	cache    map[string]*alerting.Evaluator
	Rule     *alerting.Evaluator
	Channels map[string]webhook.Endpoint
	Endpoint webhook.Endpoint
	Sender   *webhook.Sender
}

func (s *Service) Evaluate(ctx context.Context, a monitor.Assessment, plans ...monitor.Plan) (alerting.State, error) {
	if !s.Managed {
		return s.Store.EvaluateAlert(ctx, s.Rule, s.Endpoint, a, plans...)
	}
	rows, err := s.Store.AlertRuleResources(ctx, a.Monitor)
	if err != nil {
		return alerting.State{}, err
	}
	next := map[string]*alerting.Evaluator{}
	var primary alerting.State
	s.Rule = nil
	for _, v := range rows {
		key := v.UID + "/" + v.Revision + "/" + a.Monitor
		evaluator := s.cache[key]
		if evaluator == nil {
			data, err := jsonManifest(v.Manifest)
			if err != nil {
				return primary, err
			}
			rule, err := config.CompileAlert(data)
			if err != nil {
				return primary, err
			}
			evaluator, err = alerting.New(rule)
			if err != nil {
				return primary, err
			}
		}
		next[key] = evaluator
		channel := s.Endpoint
		if s.Channels != nil {
			var found bool
			channel, found = s.Channels[evaluator.Rule.Channel]
			if !found {
				continue
			}
		}
		state, err := s.Store.EvaluateAlertResource(ctx, evaluator, channel, a, v, plans...)
		if errors.Is(err, resources.ErrVersion) {
			continue
		}
		if err != nil {
			return primary, err
		}
		labels := map[string]string{}
		if len(plans) > 0 {
			labels = plans[0].Labels
		}
		if s.Rule == nil && (evaluator.Rule.Matches(a.Monitor, labels) || state.Status == "firing" || state.Status == "pending" || state.Status == "resolved") {
			s.Rule = evaluator
			primary = state
		}
	}
	s.cache = next
	return primary, nil
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

func jsonManifest(m resources.Manifest) ([]byte, error) {
	m.Metadata = resources.Metadata{Name: m.Metadata.Name}
	return json.Marshal(m)
}
