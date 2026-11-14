package monitor

import "time"

// HistoryQuery is a bounded keyset read; it never feeds the live assessment.
type HistoryQuery struct {
	Monitor, Zone string
	Since, Until  time.Time
	Before        time.Time
	BeforeID      string
	Limit         int
}
type ObservationRecord struct {
	Observation         Observation
	ReceivedAt          time.Time
	SortFinishedAt      time.Time
	DiagnosticSessionID string
}
