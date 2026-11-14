// Package operationview defines the shared read envelope for asynchronous operations.
package operationview

import (
	"context"
	"encoding/json"
	"time"
)

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type Operation struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	State      string          `json:"state"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
	Error      *Error          `json:"error"`
	Result     json.RawMessage `json:"result"`
}
type Page struct {
	Items      []Operation `json:"items"`
	NextCursor *string     `json:"nextCursor"`
}
type Reader interface {
	ReadOperation(context.Context, string) (Operation, bool, error)
	ReadOperations(context.Context, string, string, []string, int) ([]Operation, error)
}
