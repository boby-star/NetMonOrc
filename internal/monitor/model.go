package monitor

import "time"

type Check struct {
	BodyMatchesRegex string      `json:",omitempty"`
	Extract          []Extractor `json:",omitempty"`
	Type             string
	TCP              *TCPConfig
	DNS              *DNSConfig
	ICMP             *ICMPConfig
	Runner           string
	ID               string
	URL              string
	Method           string
	ExpectedStatus   []int
	MaxBodyBytes     int64
	Interval         time.Duration
	Timeout          time.Duration
	MaxAge           time.Duration
	Weight           float64
	Required         bool
}

type Plan struct {
	Diagnostics   *DiagnosticBinding `json:",omitempty"`
	Rules         []AssessmentRule
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
	DiagnosticTaskID string            `json:"diagnosticTaskID,omitempty"`
	Values           map[string]Scalar `json:"values,omitempty"`
	ExtractionStatus map[string]string `json:"extractionStatus,omitempty"`
	ID               string            `json:"id"`
	Monitor          string            `json:"monitor"`
	Revision         string            `json:"revision"`
	Check            string            `json:"check"`
	Agent            string            `json:"agent"`
	Zone             string            `json:"zone"`
	StartedAt        time.Time         `json:"startedAt"`
	FinishedAt       time.Time         `json:"finishedAt"`
	Execution        string            `json:"execution"`
	Outcome          string            `json:"outcome"`
	Reason           string            `json:"reason,omitempty"`
	StatusCode       int               `json:"statusCode,omitempty"`
}

// Diagnostic configuration is inert until its referenced profile is resolved.
type DiagnosticBinding struct {
	Enabled     bool
	ProfileRef  string
	ProfileHash string
	OnStates    []string
	When        string
	Cooldown    time.Duration
}

type ZoneAssessment struct {
	RuleErrors []string `json:"ruleErrors,omitempty"`
	Zone       string   `json:"zone"`
	State      string   `json:"state"`
	Coverage   string   `json:"coverage"`
	Q          float64  `json:"q"`
	H          *float64 `json:"h"`
	Evidence   []string `json:"evidence"`
	Reason     string   `json:"reason"`
}

type Assessment struct {
	Monitor  string           `json:"monitor"`
	Revision string           `json:"revision"`
	At       time.Time        `json:"at"`
	State    string           `json:"state"`
	Zones    []ZoneAssessment `json:"zones"`
}

// Protocol-specific parameters are separate from common orchestration fields.
type TCPConfig struct {
	Port                int
	PreferredIPProtocol string
	IPProtocolFallback  bool
}
type DNSConfig struct {
	Resolver            string
	QueryName           string
	RecordType          string
	Transport           string
	ExpectedRcodes      []string
	ExpectedRecordRegex string
}
type ICMPConfig struct {
	PreferredIPProtocol string
	IPProtocolFallback  bool
	PayloadSize         int
	DontFragment        bool
}

type AssessmentRule struct {
	ID       string
	Priority int
	When     string
	State    string
	Reason   string
}
