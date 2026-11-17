// Package diagnosticview defines the public read model for durable diagnostics.
package diagnosticview

import (
	"context"
	"time"
)

type BudgetUsed struct {
	AdditionalChecks int `json:"additionalChecks"`
}

// Explanation is a persisted terminal report projection, not a replay of the planner.
type Explanation struct {
	DurationMS   *int64        `json:"durationMs,omitempty"`
	CostUnits    float64       `json:"costUnits"`
	Decisions    []Decision    `json:"decisions"`
	Confirmation *Confirmation `json:"confirmation,omitempty"`
	BlockedSteps []string      `json:"blockedSteps,omitempty"`
	SkippedSteps []string      `json:"skippedSteps,omitempty"`
}
type Decision struct {
	ActionID         string   `json:"actionId"`
	AgentID          string   `json:"agentId"`
	AttemptNo        int      `json:"attemptNo,omitempty"`
	Phase            string   `json:"phase,omitempty"`
	HypothesesBefore []string `json:"hypothesesBefore"`
	Gain             int      `json:"gain"`
	Score            float64  `json:"score"`
	ExecutionStatus  string   `json:"executionStatus"`
	ProbeOutcome     string   `json:"probeOutcome"`
}
type Confirmation struct {
	RequiredActions        []string `json:"requiredActions"`
	ObservedActions        []string `json:"observedActions"`
	Observations           int      `json:"observations"`
	DistinctFailureDomains int      `json:"distinctFailureDomains"`
	Satisfied              bool     `json:"satisfied"`
}
type Session struct {
	ConfigRevision      string       `json:"configRevision,omitempty"`
	Explanation         *Explanation `json:"explanation,omitempty"`
	ID                  string       `json:"id"`
	MonitorID           string       `json:"monitorId"`
	ProfileRef          string       `json:"profileRef,omitempty"`
	Mode                string       `json:"mode"`
	State               string       `json:"state"`
	StartedAt           time.Time    `json:"startedAt"`
	FinishedAt          *time.Time   `json:"finishedAt"`
	RemainingHypotheses *[]string    `json:"remainingHypotheses,omitempty"`
	BudgetUsed          BudgetUsed   `json:"budgetUsed"`
	EvidenceIDs         []string     `json:"evidenceIds"`
	StopReason          *string      `json:"stopReason"`
}
type Task struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"sessionId"`
	ActionID       string    `json:"actionId"`
	AgentID        string    `json:"agentId"`
	State          string    `json:"state"`
	AttemptNo      int       `json:"attemptNo"`
	Deadline       time.Time `json:"deadline"`
	ObservationIDs []string  `json:"observationIds"`
}
type Query struct {
	Monitor, State       string
	Since, Until, Before time.Time
	BeforeID             string
	Limit                int
}
type Sessions struct {
	Items      []Session `json:"items"`
	NextCursor *string   `json:"nextCursor"`
}
type Tasks struct {
	Items      []Task  `json:"items"`
	NextCursor *string `json:"nextCursor"`
}
type Reader interface {
	DiagnosticSessions(context.Context, Query) ([]Session, error)
	DiagnosticSession(context.Context, string) (Session, bool, error)
	DiagnosticTasks(context.Context, string, string, int) ([]Task, error)
}

type Start struct {
	Monitor    string `json:"-"`
	ProfileRef string `json:"profileRef"`
	Reason     string `json:"reason,omitempty"`
	RequestID  string `json:"requestId,omitempty"`
}
