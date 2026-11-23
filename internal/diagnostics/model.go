// Package diagnostics implements deterministic planning without network or SQL.
package diagnostics

import (
	"context"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
)

type Budget struct {
	MaxChecks         int
	MaxDuration       time.Duration
	MaxParallel       int
	MaxEstimatedBytes int64
}
type Placement struct {
	Inherit    bool                    `json:",omitempty"`
	Pending    bool                    `json:"-"`
	Replicas   int                     `json:",omitempty"`
	Selector   *resourcegroup.Selector `json:",omitempty"`
	DistinctBy string                  `json:",omitempty"`
	Ref        string                  `json:",omitempty"`
	Hash       string                  `json:",omitempty"`
	AgentRefs  []string
	Zones      []string
	Strategy   string
}
type Action struct {
	Retry *Retry `json:",omitempty"`
	// AttemptNo belongs to dispatch only and must not change effective revision.
	AttemptNo int    `json:"-"`
	StepID    string `json:"-"`
	ID        string
	Check     monitor.Check
	Cost      float64
	Expected  map[string]string
	Placement Placement
}
type Retry struct {
	MaxAttempts int
}
type Outcome struct {
	Execution string
	Outcome   string
}
type TaskEvidence struct {
	ReplicaResults map[string]TaskEvidence
	AgentID        string
	Zone           string
	FailureDomain  string
	ObservationID  string
	Outcome        Outcome
	Values         map[string]monitor.Scalar
	ReasonCode     string
	ValidUntil     time.Time
}
type ContextCondition func(context.Context, map[string]TaskEvidence, FixedContext) (bool, error)

type EvidenceCondition func(context.Context, map[string]TaskEvidence, string) (bool, error)

type Step struct {
	WhenContext  ContextCondition  `json:"-"`
	WhenEvidence EvidenceCondition `json:"-"`

	ID         string
	ActionID   string
	DependsOn  []string
	When       func(context.Context, map[string]Outcome, string) (bool, error) `json:"-"`
	Expression string                                                          `json:",omitempty"`
}
type Confirmation struct {
	ActionRefs                []string
	MinObservations           int
	MinDistinctFailureDomains int
	MinSpacing                time.Duration
}
type Profile struct {
	MaxNoGainSteps      int           `json:",omitempty"`
	Confirmation        *Confirmation `json:",omitempty"`
	ContinueAfterUnique bool          `json:",omitempty"`
	Name                string
	Mode                string
	Budget              Budget
	Hypotheses          []string
	Actions             []Action
	Steps               []Step
}
type Executor struct {
	Labels map[string]string
	// Evidence before this trusted topology epoch cannot establish independence.
	EvidenceNotBefore time.Time
	ID                string
	Zone              string
	FailureDomain     string
	Online            bool
	Capacity          int
	Active            int
	Supported         map[string]bool
}
type Evidence struct {
	FallbackRunID         string
	FallbackTaskID        string
	FallbackOriginAgentID string
	CheckID               string
	ConfigRevision        string
	FinishedAt            time.Time
	ObservationID         string
	ValidUntil            time.Time
	ActionID              string
	AgentID               string
	Outcome               Outcome
}

// BaselineEvidence records regular observations supplied to the planner. It is
// a historical input snapshot, not a claim that every input remained usable.
type BaselineEvidence struct {
	FallbackRunID         string    `json:"fallbackRunId,omitempty"`
	FallbackTaskID        string    `json:"fallbackTaskId,omitempty"`
	FallbackOriginAgentID string    `json:"fallbackOriginAgentId,omitempty"`
	ObservationID         string    `json:"observationId"`
	ActionID              string    `json:"actionId"`
	CheckID               string    `json:"checkId"`
	AgentID               string    `json:"agentId"`
	Zone                  string    `json:"zone"`
	ConfigRevision        string    `json:"configRevision"`
	FailureDomain         string    `json:"failureDomain,omitempty"`
	FinishedAt            time.Time `json:"finishedAt"`
	ValidUntil            time.Time `json:"validUntil"`
	ExecutionStatus       string    `json:"executionStatus"`
	ProbeOutcome          string    `json:"probeOutcome"`
}
type Decision struct {
	TaskEvidence     TaskEvidence `json:"-"`
	StepID           string       `json:"stepId,omitempty"`
	ObservationID    string       `json:"observationId,omitempty"`
	FinishedAt       *time.Time   `json:"finishedAt,omitempty"`
	ValidUntil       *time.Time   `json:"validUntil,omitempty"`
	FailureDomain    string       `json:"failureDomain,omitempty"`
	AttemptNo        int          `json:"attemptNo,omitempty"`
	Phase            string       `json:"phase,omitempty"`
	ActionID         string       `json:"actionId"`
	AgentID          string       `json:"agentId"`
	HypothesesBefore []string     `json:"hypothesesBefore"`
	Gain             int          `json:"gain"`
	Score            float64      `json:"score"`
	Outcome          Outcome      `json:"result"`
}
type EvidencePolicy struct {
	MaxAgeMS  int64 `json:"maxAgeMs"`
	MaxSkewMS int64 `json:"maxSkewMs"`
}
type Result struct {
	EstimatedBytes   *int64              `json:"estimatedBytes,omitempty"`
	BaselineEvidence *[]BaselineEvidence `json:"baselineEvidence,omitempty"`
	EvidencePolicy   *EvidencePolicy     `json:"evidencePolicy,omitempty"`
	Confirmation     *ConfirmationResult `json:"confirmation,omitempty"`
	BlockedSteps     []string            `json:"blockedSteps,omitempty"`
	DurationMillis   *int64              `json:"durationMs,omitempty"`
	Mode             string              `json:"mode"`
	Status           string              `json:"status"`
	Reason           string              `json:"reason"`
	Hypotheses       []string            `json:"hypotheses"`
	Attempts         int                 `json:"attempts"`
	Cost             float64             `json:"costUnits"`
	Decisions        []Decision          `json:"decisions"`
	SkippedSteps     []string            `json:"skippedSteps,omitempty"`
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
	noGainSteps       int
}
type Runner interface {
	Execute(context.Context, Action, Executor) (Outcome, error)
}

// AdmissionRunner reserves execution capacity before the planner spends budget.
// The returned runner owns the reservation until release, including retries.
type AdmissionRunner interface {
	Admit(context.Context, Action, Executor) (Runner, func(), error)
}

// ObservedOutcome carries the committed Observation's time, not delivery time.
type ObservedOutcome struct {
	Values        map[string]monitor.Scalar
	ReasonCode    string
	Outcome       Outcome
	ObservationID string
	FinishedAt    time.Time
}
type EvidenceRunner interface {
	ExecuteEvidence(context.Context, Action, Executor) (ObservedOutcome, error)
}
type Engine struct {
	CELContext      *SessionContext
	EvidenceMaxAge  time.Duration
	EvidenceMaxSkew time.Duration
	// TaskOverhead reserves orchestration time inside the same global deadline;
	// it never increases the driver's configured network probe timeout.
	TaskOverhead time.Duration
	Runner       Runner
	// Eligibility is a required Controller-owned policy gate, not a permission
	// inferred from score or from the Agent's self-reported reachability.
	Eligible      func(Action, Executor) bool
	EstimateBytes func(Action, Executor) (int64, bool)
}

// Report separates durable budget reservations from known probe outcomes.
type Report struct {
	ReservedEstimatedBytes *int64     `json:"reservedEstimatedBytes,omitempty"`
	ID                     string     `json:"id"`
	Monitor                string     `json:"monitor"`
	Revision               string     `json:"revision"`
	ReservedChecks         int        `json:"reservedChecks"`
	StartedAt              time.Time  `json:"startedAt"`
	FinishedAt             *time.Time `json:"finishedAt,omitempty"`
	Result                 Result     `json:"result"`
}
