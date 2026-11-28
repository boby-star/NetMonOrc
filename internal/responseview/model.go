package responseview

import (
	"encoding/json"
	"time"
)

type Event struct {
	ID          string          `json:"id"`
	Monitor     string          `json:"monitor"`
	Revision    string          `json:"revision"`
	RuleID      string          `json:"ruleId"`
	Agent       string          `json:"agent"`
	Status      string          `json:"status"`
	Reason      string          `json:"reason"`
	CreatedAt   time.Time       `json:"createdAt"`
	AcceptUntil time.Time       `json:"acceptUntil"`
	BlockUntil  time.Time       `json:"blockUntil"`
	Assessment  json.RawMessage `json:"assessment"`
	Rule        json.RawMessage `json:"rule"`
}
