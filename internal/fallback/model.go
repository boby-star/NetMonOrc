package fallback

import (
	"context"
	"time"
)

type Run struct {
	ID          string    `json:"id"`
	Monitor     string    `json:"monitor"`
	Revision    string    `json:"revision"`
	Check       string    `json:"check"`
	OriginID    string    `json:"originObservationId"`
	OriginAgent string    `json:"originAgent"`
	OriginZone  string    `json:"originZone"`
	Cause       string    `json:"cause"`
	Strategy    string    `json:"strategy"`
	MaxAttempts int       `json:"maxAttempts"`
	StartedAt   time.Time `json:"startedAt"`
	Deadline    time.Time `json:"deadline"`
	State       string    `json:"state"`
	Used        int       `json:"usedAttempts"`
	Tasks       []Task    `json:"tasks"`
}
type Task struct {
	ID            string    `json:"id"`
	Agent         string    `json:"agent"`
	Zone          string    `json:"zone"`
	Attempt       int       `json:"attempt"`
	State         string    `json:"state"`
	Deadline      time.Time `json:"deadline"`
	ObservationID string    `json:"observationId"`
}

type Reader interface {
	ReadFallback(context.Context, string) (Run, bool, error)
	FallbackRuns(context.Context, string, string, int) ([]Run, error)
}
