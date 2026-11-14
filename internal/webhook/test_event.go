package webhook

import "time"

// TestEvent is distinct from Alert events and intentionally carries no message.
type TestEvent struct {
	ID     string    `json:"event_id"`
	Type   string    `json:"event_type"`
	At     time.Time `json:"occurred_at"`
	Schema string    `json:"schema_version"`
}
type TestResult struct {
	EventID        string `json:"eventId"`
	DeliveryState  string `json:"deliveryState"`
	Attempts       int    `json:"attempts"`
	LastHTTPStatus *int   `json:"lastHttpStatus"`
}
