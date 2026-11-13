package webhook

import "time"

// Summary is a read-only database snapshot, not an acknowledgement of future
// deliveries. Counts are scoped to one endpoint revision, rule and Monitor.
type Summary struct {
	Enabled   bool         `json:"enabled"`
	Endpoint  string       `json:"endpoint"`
	Monitor   string       `json:"monitor"`
	Rule      string       `json:"rule"`
	At        time.Time    `json:"at"`
	Pending   int64        `json:"pending"`
	Leased    int64        `json:"leased"`
	Delivered int64        `json:"delivered"`
	Failed    int64        `json:"failed"`
	Cancelled int64        `json:"cancelled"`
	Latest    *EventStatus `json:"latest"`
}

type EventStatus struct {
	ID         string `json:"id"`
	State      string `json:"state"`
	Attempts   int    `json:"attempts"`
	StatusCode int    `json:"statusCode"`
}
