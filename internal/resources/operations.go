package resources

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/auth"
)

var ErrPreconditionRequired = errors.New("expected resource version required")
var ErrIdempotency = errors.New("idempotency key reused for another request")

const IdempotencyTTL = 24 * time.Hour

type OperationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type ApplyTarget struct {
	Kind              string `json:"kind"`
	Name              string `json:"name"`
	UID               string `json:"uid"`
	ResourceVersion   string `json:"resourceVersion"`
	EffectiveRevision string `json:"effectiveRevision"`
	Ready             bool   `json:"ready"`
	Reason            string `json:"reason"`
}
type ApplyResult struct {
	DesiredCommitted bool          `json:"desiredCommitted"`
	Targets          []ApplyTarget `json:"targets"`
}
type Operation struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	State      string          `json:"state"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
	Error      *OperationError `json:"error"`
	Result     ApplyResult     `json:"result"`
}
type OperationPage struct {
	Items      []Operation `json:"items"`
	NextCursor *string     `json:"nextCursor"`
}

func RequestDigest(batch Batch) []byte {
	data, _ := json.Marshal(batch)
	sum := sha256.Sum256(canonicalJSON(data))
	return sum[:]
}
func (s *Service) Apply(ctx context.Context, actor auth.AdminCredential, batch Batch) (Operation, error) {
	if len(batch.RequestID) > 128 || !utf8.ValidString(batch.RequestID) || strings.ContainsFunc(batch.RequestID, unicode.IsControl) {
		return Operation{}, ErrInvalid
	}
	digest := RequestDigest(batch)
	if batch.RequestID != "" {
		prior, found, err := s.Repo.RecoverApply(ctx, actor, batch.RequestID, digest)
		if err != nil {
			return prior, err
		}
		if found {
			return prior, nil
		}
	}
	if len(batch.Manifests) < 1 || len(batch.Manifests) > 20 || len(batch.ExpectedVersions) > len(batch.Manifests) {
		return Operation{}, ErrInvalid
	}
	keys := map[string]bool{}
	for _, m := range batch.Manifests {
		key := m.Kind + "/" + m.Metadata.Name
		if keys[key] {
			return Operation{}, ErrInvalid
		}
		keys[key] = true
	}
	for key, version := range batch.ExpectedVersions {
		if !keys[key] || (version != nil && (len(*version) == 0 || len(*version) > 200)) {
			return Operation{}, ErrInvalid
		}
	}
	if !s.Validate(batch).Valid {
		return Operation{}, ErrInvalid
	}
	revisions := map[string]string{}
	for _, m := range batch.Manifests {
		p, err := s.Registry.Compile(m)
		if err != nil {
			return Operation{}, ErrInvalid
		}
		revisions[m.Kind+"/"+m.Metadata.Name] = p.Revision
	}
	return s.Repo.ApplyResources(ctx, actor, batch, revisions, digest)
}
