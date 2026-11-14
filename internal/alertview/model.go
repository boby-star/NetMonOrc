// Package alertview defines sanitized public projections of alert lifecycle data.
package alertview

import (
	"context"
	"time"
)

type Alert struct {
	ID             string     `json:"id"`
	MonitorID      string     `json:"monitorId"`
	RuleRef        string     `json:"ruleRef"`
	Zone           *string    `json:"zone"`
	State          string     `json:"state"`
	Severity       string     `json:"severity"`
	ReasonCodes    []string   `json:"reasonCodes"`
	EvidenceIDs    []string   `json:"evidenceIds"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	FiredAt        *time.Time `json:"firedAt"`
	ResolvedAt     *time.Time `json:"resolvedAt"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt"`
}
type History struct {
	SortAt         time.Time `json:"-"`
	ID             string    `json:"id"`
	From           string    `json:"from"`
	To             string    `json:"to"`
	At             time.Time `json:"at"`
	ConfigRevision string    `json:"configRevision"`
	Suppressed     bool      `json:"suppressed"`
}
type Delivery struct {
	ID             string     `json:"id"`
	EventID        string     `json:"eventId"`
	State          string     `json:"state"`
	Attempts       int        `json:"attempts"`
	LastHTTPStatus *int       `json:"lastHttpStatus"`
	LastAttemptAt  *time.Time `json:"lastAttemptAt"`
}
type Query struct {
	BeforeAt                                             time.Time
	State, Severity, Monitor, Zone, Before, EndpointHash string
	Limit                                                int
}
type AlertPage struct {
	Items      []Alert `json:"items"`
	NextCursor *string `json:"nextCursor"`
}
type HistoryPage struct {
	Items      []History `json:"items"`
	NextCursor *string   `json:"nextCursor"`
}
type DeliveryPage struct {
	Items      []Delivery `json:"items"`
	NextCursor *string    `json:"nextCursor"`
}
type Reader interface {
	Alerts(context.Context, Query) ([]Alert, error)
	Alert(context.Context, string) (Alert, bool, error)
	AlertHistory(context.Context, string, Query) ([]History, error)
	WebhookDeliveries(context.Context, Query) ([]Delivery, error)
}
