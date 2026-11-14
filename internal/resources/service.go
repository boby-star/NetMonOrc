package resources

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type Service struct {
	Repo     Repository
	Registry Registry
	slots    chan struct{}
}

func New(repo Repository, registry Registry) *Service {
	return &Service{Repo: repo, Registry: registry, slots: make(chan struct{}, 1)}
}
func (s *Service) Admit() bool {
	select {
	case s.slots <- struct{}{}:
		return true
	default:
		return false
	}
}
func (s *Service) Release() { <-s.slots }
func (s *Service) Status(v View, activeRevision string) View {
	status := &Status{State: "pending", DesiredRevision: v.Revision, Conditions: []Condition{{Type: "Reconciled", Status: "False", Reason: "controller_not_selected"}}}
	if v.Manifest.Metadata.Name == s.Registry.Active {
		status.Conditions[0].Reason = "revision_pending"
		if v.Revision == activeRevision {
			status.State = "active"
			status.Conditions[0] = Condition{Type: "Reconciled", Status: "True", Reason: "controller_loaded"}
		}
	}
	v.Status = status
	return v
}

type Detail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}
type Batch struct {
	Manifests        []Manifest         `json:"manifests"`
	ExpectedVersions map[string]*string `json:"expectedVersions,omitempty"`
	RequestID        string             `json:"requestId,omitempty"`
}
type Validation struct {
	Valid             bool           `json:"valid"`
	Errors            []Detail       `json:"errors"`
	Warnings          []Detail       `json:"warnings"`
	NormalizedSummary map[string]any `json:"normalizedSummary"`
}

func (s *Service) Validate(batch Batch) Validation {
	result := Validation{Valid: true, Errors: []Detail{}, Warnings: []Detail{}, NormalizedSummary: map[string]any{}}
	if len(batch.Manifests) < 1 || len(batch.Manifests) > 20 {
		result.Valid = false
		result.Errors = append(result.Errors, Detail{Code: "BATCH_LIMIT", Message: "expected 1..20 Monitor manifests"})
		return result
	}
	seen := map[string]bool{}
	revisions := map[string]string{}
	for i, m := range batch.Manifests {
		key := m.Kind + "/" + m.Metadata.Name
		p, err := s.Registry.Compile(m)
		if seen[key] {
			err = fmt.Errorf("duplicate resource in batch")
		}
		seen[key] = true
		if err != nil {
			result.Valid = false
			result.Errors = append(result.Errors, Detail{Code: "INVALID_MANIFEST", Message: err.Error(), Path: fmt.Sprintf("manifests[%d]", i)})
			continue
		}
		revisions[key] = p.Revision
		if m.Metadata.Name != s.Registry.Active {
			result.Warnings = append(result.Warnings, Detail{Code: "MONITOR_NOT_SELECTED", Message: key + " requires selection by Controller configuration"})
		}
	}
	result.NormalizedSummary["effectiveRevisions"] = revisions
	return result
}

type Diff struct {
	Kind              string  `json:"kind"`
	Name              string  `json:"name"`
	Action            string  `json:"action"`
	ResourceVersion   *string `json:"resourceVersion"`
	EffectiveRevision string  `json:"effectiveRevision"`
}
type Plan struct {
	PlanID     string   `json:"planId"`
	Diffs      []Diff   `json:"diffs"`
	Placements []any    `json:"placements"`
	Warnings   []Detail `json:"warnings"`
}

func (s *Service) Plan(ctx context.Context, batch Batch) (Plan, error) {
	v := s.Validate(batch)
	if !v.Valid {
		return Plan{}, ErrInvalid
	}
	plan := Plan{Diffs: []Diff{}, Placements: []any{}, Warnings: v.Warnings}
	for _, m := range batch.Manifests {
		current, found, err := s.Repo.Resource(ctx, m.Metadata.Name)
		if err != nil {
			return plan, err
		}
		if expected, provided := batch.ExpectedVersions[m.Kind+"/"+m.Metadata.Name]; provided {
			if expected == nil && found {
				return plan, ErrConflict
			}
			if expected != nil && (!found || current.ResourceVersion != *expected) {
				return plan, ErrVersion
			}
		}
		compiled, _ := s.Registry.Compile(m)
		diff := Diff{Kind: m.Kind, Name: m.Metadata.Name, Action: "create", EffectiveRevision: compiled.Revision}
		if found {
			version := current.ResourceVersion
			diff.ResourceVersion = &version
			diff.Action = "replace"
			old, _ := json.Marshal(current.Manifest)
			next, _ := json.Marshal(m)
			if bytes.Equal(canonicalJSON(old), canonicalJSON(next)) {
				diff.Action = "unchanged"
			}
		}
		plan.Diffs = append(plan.Diffs, diff)
	}
	// Actual placement is the fixed roster; a dry-run does not invent live capacity.
	plan.Warnings = append(plan.Warnings, Detail{Code: "FIXED_ROSTER", Message: "Placement remains the configured fixed Agent roster; unselected Monitors await Controller selection"})
	b, _ := json.Marshal(plan)
	digest := sha256.Sum256(b)
	plan.PlanID = hex.EncodeToString(digest[:])
	return plan, nil
}

func canonicalJSON(data []byte) []byte {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return data
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return data
	}
	return encoded
}
