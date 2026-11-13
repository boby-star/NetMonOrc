package monitor

import "time"

type Check struct {
	ID             string
	URL            string
	Method         string
	ExpectedStatus []int
	MaxBodyBytes   int64
	Interval       time.Duration
	Timeout        time.Duration
	MaxAge         time.Duration
	Weight         float64
	Required       bool
}

type Plan struct {
	Name          string
	TargetAddress string
	Revision      string
	Checks        []Check
	Zones         []string
	MaxAge        time.Duration
	MaxSkew       time.Duration
	MinCoverage   float64
	HealthyMin    float64
	DegradedMin   float64
}

// Slot is an expected observation source, never inferred from received data.
type Slot struct {
	Agent string
	Zone  string
}

type Observation struct {
	ID         string    `json:"id"`
	Monitor    string    `json:"monitor"`
	Revision   string    `json:"revision"`
	Check      string    `json:"check"`
	Agent      string    `json:"agent"`
	Zone       string    `json:"zone"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt"`
	Execution  string    `json:"execution"`
	Outcome    string    `json:"outcome"`
	Reason     string    `json:"reason,omitempty"`
	StatusCode int       `json:"statusCode,omitempty"`
}

type ZoneAssessment struct {
	Zone     string   `json:"zone"`
	State    string   `json:"state"`
	Coverage string   `json:"coverage"`
	Q        float64  `json:"q"`
	H        *float64 `json:"h"`
	Evidence []string `json:"evidence"`
	Reason   string   `json:"reason"`
}

type Assessment struct {
	Monitor  string           `json:"monitor"`
	Revision string           `json:"revision"`
	At       time.Time        `json:"at"`
	State    string           `json:"state"`
	Zones    []ZoneAssessment `json:"zones"`
}
