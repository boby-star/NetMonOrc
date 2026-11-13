// Package diagnostics implements deterministic planning without network or SQL.
package diagnostics

import (
	"context"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

type Budget struct {
	MaxChecks         int
	MaxDuration       time.Duration
	MaxParallel       int
	MaxEstimatedBytes int64
}
type Placement struct {
	AgentRefs []string
	Zones     []string
	Strategy  string
}
type Action struct {
	ID        string
	Check     monitor.Check
	Cost      float64
	Expected  map[string]string
	Placement Placement
}
type Outcome struct {
	Execution string
	Outcome   string
}
type Step struct {
	ID        string
	ActionID  string
	DependsOn []string
	When      func(context.Context, map[string]Outcome, string) (bool, error)
}
type Profile struct {
	Name       string
	Mode       string
	Budget     Budget
	Hypotheses []string
	Actions    []Action
	Steps      []Step
}
type Executor struct {
	ID            string
	Zone          string
	FailureDomain string
	Online        bool
	Capacity      int
	Active        int
	Supported     map[string]bool
}
type Evidence struct {
	ActionID string
	AgentID  string
	Outcome  Outcome
}
type Decision struct {
	ActionID         string   `json:"actionId"`
	AgentID          string   `json:"agentId"`
	HypothesesBefore []string `json:"hypothesesBefore"`
	Gain             int      `json:"gain"`
	Score            float64  `json:"score"`
	Outcome          Outcome  `json:"result"`
}
type Result struct {
	Mode         string     `json:"mode"`
	Status       string     `json:"status"`
	Reason       string     `json:"reason"`
	Hypotheses   []string   `json:"hypotheses"`
	Attempts     int        `json:"attempts"`
	Cost         float64    `json:"costUnits"`
	Decisions    []Decision `json:"decisions"`
	SkippedSteps []string   `json:"skippedSteps,omitempty"`
}

// The coordinator must reuse the same ledger for an incident episode. This
// initial implementation is in memory; it is not a durable public start API.
type Ledger struct {
	mu                sync.Mutex
	running           bool
	StartedAt         time.Time
	UsedChecks        int
	EstimatedBytes    int64
	policy            *Budget
	adaptiveAttempted map[string]bool
}
type Runner interface {
	Execute(context.Context, Action, Executor) (Outcome, error)
}
type Engine struct {
	Runner Runner
	// Eligibility is a required Controller-owned policy gate, not a permission
	// inferred from score or from the Agent's self-reported reachability.
	Eligible      func(Action, Executor) bool
	EstimateBytes func(Action, Executor) (int64, bool)
}
