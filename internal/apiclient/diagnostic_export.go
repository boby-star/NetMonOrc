package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
)

// DiagnosticExport preserves exact response numbers, including int64 values.
// Reads are sequential, not an atomic database snapshot. Missing retention
// evidence is explicit; authorization and transport failures abort the export.
type DiagnosticExport struct {
	BaselineEvidenceKnown bool              `json:"baselineEvidenceKnown"`
	Schema                int               `json:"schema"`
	CollectedAt           time.Time         `json:"collectedAt"`
	Complete              bool              `json:"complete"`
	Session               json.RawMessage   `json:"session"`
	Tasks                 []json.RawMessage `json:"tasks"`
	Observations          []json.RawMessage `json:"observations"`
	MissingObservationIDs []string          `json:"missingObservationIds"`
}

func (c *Client) ExportDiagnostic(ctx context.Context, id string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if id == "" || len(id) > 128 {
		return nil, fmt.Errorf("one diagnostic session ID required")
	}
	route := "/api/v1/diagnostics/" + url.PathEscape(id)
	read := func(path string) ([]byte, error) {
		for {
			b, err := c.JSON(ctx, "GET", path, nil)
			var api *APIError
			if !errors.As(err, &api) || (api.Status != 429 && api.Status != 503) {
				return b, err
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
	first, err := read(route)
	if err != nil {
		return nil, err
	}
	var session diagnosticview.Session
	if err = json.Unmarshal(first, &session); err != nil || session.ID != id {
		return nil, fmt.Errorf("unexpected diagnostic session")
	}
	if session.State != "completed" && session.State != "inconclusive" && session.State != "cancelled" {
		return nil, fmt.Errorf("wait for a terminal diagnostic session before exporting")
	}
	out := DiagnosticExport{Schema: 1, Session: first, Tasks: []json.RawMessage{}, Observations: []json.RawMessage{}, MissingObservationIDs: []string{}}
	observationIDs := map[string]bool{}
	ids := []string{}
	add := func(id string) {
		if id != "" && !observationIDs[id] {
			observationIDs[id] = true
			ids = append(ids, id)
		}
	}
	for _, id := range session.EvidenceIDs {
		add(id)
	}
	baseline := map[string]diagnosticview.BaselineEvidence{}
	if session.Explanation != nil && session.Explanation.BaselineEvidence != nil {
		out.BaselineEvidenceKnown = true
		for _, b := range *session.Explanation.BaselineEvidence {
			if b.ObservationID == "" || b.ActionID == "" || b.CheckID == "" || b.AgentID == "" || b.Zone == "" || b.ConfigRevision == "" || b.ConfigRevision != session.ConfigRevision || b.FinishedAt.IsZero() || !b.ValidUntil.After(b.FinishedAt) {
				return nil, fmt.Errorf("invalid baseline evidence provenance")
			}
			if old, ok := baseline[b.ObservationID]; ok {
				// A physical sample can match several equivalent catalog actions.
				old.ActionID = b.ActionID
				if old != b {
					return nil, fmt.Errorf("conflicting baseline evidence provenance")
				}
			}
			baseline[b.ObservationID] = b
			add(b.ObservationID)
		}
	}
	expectedTasks := map[string]diagnosticview.Task{}
	seenTasks := map[string]bool{}
	seenCursors := map[string]bool{}
	cursor := ""
	size := len(first)
	for {
		path := route + "/tasks?limit=200"
		if cursor != "" {
			path += "&cursor=" + url.QueryEscape(cursor)
		}
		b, err := read(path)
		if err != nil {
			return nil, err
		}
		var page struct {
			Items      []json.RawMessage `json:"items"`
			NextCursor *string           `json:"nextCursor"`
		}
		if json.Unmarshal(b, &page) != nil {
			return nil, fmt.Errorf("invalid task page")
		}
		for _, raw := range page.Items {
			var task diagnosticview.Task
			if json.Unmarshal(raw, &task) != nil || task.ID == "" || task.SessionID != id || seenTasks[task.ID] {
				return nil, fmt.Errorf("unexpected or repeated diagnostic task")
			}
			seenTasks[task.ID] = true
			if len(seenTasks) > 1000 {
				return nil, fmt.Errorf("export exceeds 1000 tasks")
			}
			out.Tasks = append(out.Tasks, raw)
			size += len(raw)
			for _, id := range task.ObservationIDs {
				if previous, ok := expectedTasks[id]; ok && previous.ID != task.ID {
					return nil, fmt.Errorf("evidence belongs to multiple tasks")
				}
				expectedTasks[id] = task
				add(id)
			}
		}
		if size > 32<<20 || len(ids) > 10000 {
			return nil, fmt.Errorf("diagnostic export too large")
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
		if cursor == "" || seenCursors[cursor] || len(page.Items) == 0 {
			return nil, fmt.Errorf("task pagination did not advance")
		}
		seenCursors[cursor] = true
	}
	for _, observationID := range ids {
		b, err := read("/api/v1/observations/" + url.PathEscape(observationID))
		var api *APIError
		if errors.As(err, &api) && api.Status == 404 {
			out.MissingObservationIDs = append(out.MissingObservationIDs, observationID)
			continue
		}
		if err != nil {
			return nil, err
		}
		var o struct {
			ID         string    `json:"observationId"`
			Monitor    string    `json:"monitorId"`
			Session    string    `json:"diagnosticSessionId"`
			Task       string    `json:"diagnosticTaskId"`
			Agent      string    `json:"agentId"`
			Check      string    `json:"checkId"`
			Zone       string    `json:"networkZone"`
			Revision   string    `json:"configRevision"`
			FinishedAt time.Time `json:"finishedAt"`
			Execution  string    `json:"executionStatus"`
			Outcome    string    `json:"probeOutcome"`
		}
		if json.Unmarshal(b, &o) != nil || o.ID != observationID || o.Monitor != session.MonitorID {
			return nil, fmt.Errorf("observation provenance does not match diagnostic session")
		}
		if proof, ok := baseline[observationID]; ok {
			if _, task := expectedTasks[observationID]; task || o.Session != "" || o.Task != "" || o.Agent != proof.AgentID || o.Check != proof.CheckID || o.Zone != proof.Zone || o.Revision != proof.ConfigRevision || !o.FinishedAt.Equal(proof.FinishedAt) || o.Execution != proof.ExecutionStatus || o.Outcome != proof.ProbeOutcome {
				return nil, fmt.Errorf("observation provenance does not match regular baseline")
			}
		} else if o.Session != id {
			return nil, fmt.Errorf("observation provenance does not match diagnostic session")
		}
		if task, ok := expectedTasks[observationID]; ok && (o.Task != task.ID || o.Agent != task.AgentID) {
			return nil, fmt.Errorf("observation provenance does not match physical task")
		}
		size += len(b)
		if size > 32<<20 {
			return nil, fmt.Errorf("diagnostic export exceeds 32 MiB")
		}
		out.Observations = append(out.Observations, b)
	}
	last, err := read(route)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(first, last) {
		return nil, fmt.Errorf("diagnostic session changed during export; retry")
	}
	out.Complete = len(out.MissingObservationIDs) == 0
	out.CollectedAt = time.Now().UTC()
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(b) > 32<<20 {
		return nil, fmt.Errorf("diagnostic export exceeds 32 MiB")
	}
	return b, nil
}
