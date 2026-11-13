// Package alerting evaluates alert lifecycle without HTTP or SQL dependencies.
package alerting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"reflect"
	"time"
)

type Rule struct {
	Name         string
	Monitor      string
	Scope        string
	When         string
	ResolveWhen  string
	Severity     string
	For          time.Duration
	Cooldown     time.Duration
	Channel      string
	NotifyEvents []string
	Enabled      bool
	Description  string
}
type MonitorView struct {
	Name          string `cel:"name"`
	State         string `cel:"state"`
	CoverageState string `cel:"coverageState"`
}
type Evaluator struct {
	Rule          Rule
	Hash          string
	when, resolve cel.Program
	startedAt     time.Time
}

func New(r Rule) (*Evaluator, error) {
	env, e := cel.NewEnv(ext.NativeTypes(reflect.TypeOf(MonitorView{}), ext.ParseStructTags(true)), cel.Variable("monitor", cel.ObjectType("alerting.MonitorView")))
	if e != nil {
		return nil, e
	}
	compile := func(s string) (cel.Program, error) {
		if len(s) == 0 || len(s) > 2048 {
			return nil, fmt.Errorf("invalid alert expression length")
		}
		ast, issues := env.Compile(s)
		if issues.Err() != nil {
			return nil, issues.Err()
		}
		if ast.OutputType() != cel.BoolType {
			return nil, fmt.Errorf("alert expression must return bool")
		}
		return env.Program(ast, cel.CostLimit(1024), cel.InterruptCheckFrequency(50))
	}
	when, e := compile(r.When)
	if e != nil {
		return nil, e
	}
	resolve, e := compile(r.ResolveWhen)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256(b)
	return &Evaluator{Rule: r, Hash: hex.EncodeToString(sum[:]), when: when, resolve: resolve, startedAt: time.Now()}, nil
}

type State struct {
	Status               string     `json:"status"`
	RuleHash             string     `json:"ruleHash"`
	Revision             string     `json:"revision"`
	PendingSince         *time.Time `json:"pendingSince,omitempty"`
	Armed                bool       `json:"armed"`
	LastEvaluatedAt      time.Time  `json:"lastEvaluatedAt"`
	LastTransitionAt     time.Time  `json:"lastTransitionAt"`
	LastFiringQueuedAt   *time.Time `json:"lastFiringQueuedAt,omitempty"`
	LastResolvedQueuedAt *time.Time `json:"lastResolvedQueuedAt,omitempty"`
}
type Transition struct {
	From     string    `json:"from"`
	To       string    `json:"to"`
	At       time.Time `json:"at"`
	Revision string    `json:"revision"`
}
type Evaluation struct {
	State      State
	Transition *Transition
	Notify     string
	Suppressed bool
}

func (e *Evaluator) Step(ctx context.Context, s State, a monitor.Assessment) Evaluation {
	now := a.At
	if s.Status == "" {
		s.Status = "normal"
		s.Armed = true
	}
	if !now.After(s.LastEvaluatedAt) {
		return Evaluation{State: s}
	}
	from := s.Status
	// A gap over three 1s controller ticks cannot prove continuous pending.
	continuous := s.LastEvaluatedAt.IsZero() || now.Sub(s.LastEvaluatedAt) <= 3*time.Second
	if s.Status == "pending" && (s.PendingSince == nil || s.LastEvaluatedAt.Before(e.startedAt) || !continuous || s.RuleHash != e.Hash || s.Revision != a.Revision) {
		s.Status = "normal"
		s.PendingSince = nil
	}
	s.RuleHash = e.Hash
	s.Revision = a.Revision
	s.LastEvaluatedAt = now
	coverage := "sufficient"
	evidence := 0
	for _, z := range a.Zones {
		evidence += len(z.Evidence)
		if z.Coverage != "complete" {
			coverage = "degraded"
		}
	}
	if evidence == 0 {
		coverage = "unknown"
	}
	c, done := context.WithTimeout(ctx, 25*time.Millisecond)
	defer done()
	eval := func(p cel.Program) *bool {
		v, _, err := p.ContextEval(c, map[string]any{"monitor": MonitorView{a.Monitor, a.State, coverage}})
		if err != nil {
			return nil
		}
		b, ok := v.Value().(bool)
		if !ok {
			return nil
		}
		return &b
	}
	when, resolve := eval(e.when), eval(e.resolve)
	yes := func(v *bool) bool { return v != nil && *v }
	if s.Status == "firing" {
		if yes(resolve) {
			s.Status = "resolved"
			s.PendingSince = nil
			s.Armed = when != nil && !*when
		}
	} else {
		if s.Status == "resolved" && when != nil && !*when {
			s.Armed = true
		}
		allowed := e.Rule.Enabled && (s.Status != "resolved" || s.Armed)
		if !allowed || !yes(when) {
			if s.Status == "pending" {
				s.Status = "normal"
				s.PendingSince = nil
			}
		} else {
			if s.Status != "pending" {
				s.Status = "pending"
				t := now
				s.PendingSince = &t
			}
			if now.Sub(*s.PendingSince) >= e.Rule.For {
				s.Status = "firing"
				s.PendingSince = nil
				s.Armed = false
			}
		}
	}
	result := Evaluation{State: s}
	if s.Status == from {
		return result
	}
	result.State.LastTransitionAt = now
	result.Transition = &Transition{From: from, To: s.Status, At: now, Revision: a.Revision}
	if s.Status != "firing" && s.Status != "resolved" {
		return result
	}
	for _, kind := range e.Rule.NotifyEvents {
		if kind == s.Status {
			result.Notify = kind
		}
	}
	last := s.LastFiringQueuedAt
	if s.Status == "resolved" {
		last = s.LastResolvedQueuedAt
	}
	result.Suppressed = last != nil && now.Sub(*last) < e.Rule.Cooldown
	return result
}

type Event struct {
	ID       string    `json:"event_id"`
	Type     string    `json:"event_type"`
	Rule     string    `json:"alert_rule"`
	Monitor  string    `json:"monitor"`
	Zone     *string   `json:"zone"`
	State    string    `json:"state"`
	Severity string    `json:"severity"`
	Reasons  []string  `json:"reason_codes"`
	Evidence []string  `json:"evidence_ids"`
	At       time.Time `json:"occurred_at"`
	Schema   string    `json:"schema_version"`
}
