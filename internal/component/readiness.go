package component

import "time"

const ReadinessMaxAge = 15 * time.Second
const ReadinessFutureSkew = 5 * time.Second

type Readiness struct {
	Revision  string
	Ref       string
	Hash      string
	State     string
	Reason    string
	CheckedAt time.Time
}

func Fresh(at, now time.Time) bool {
	return !at.IsZero() && now.Sub(at) <= ReadinessMaxAge && at.Sub(now) <= ReadinessFutureSkew
}

// Status is a safe live API projection; unknown never becomes ready by default.
type Status struct {
	MonitorID    string     `json:"monitorId"`
	Revision     string     `json:"revision"`
	ComponentRef string     `json:"componentRef"`
	State        string     `json:"state"`
	Reason       string     `json:"reason"`
	CheckedAt    *time.Time `json:"checkedAt"`
}
