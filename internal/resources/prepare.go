package resources

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"maps"
)

// PreparedConfiguration keeps JSON numeric spelling inside Source. Passing
// manifests through browser JSON.parse/stringify would turn typed 1.0 into 1.
type PreparedConfiguration struct {
	Source        string `json:"source"`
	RequestID     string `json:"requestId"`
	ResourceCount int    `json:"resourceCount"`
}

// Prepare only reads the catalog. Explicit preconditions and request IDs survive
// unchanged; absent versions are captured for later optimistic apply, not locked.
func (s *Service) Prepare(ctx context.Context, batch Batch) (PreparedConfiguration, error) {
	if !validRequestID(batch.RequestID) {
		return PreparedConfiguration{}, ErrInvalid
	}
	batch.ExpectedVersions = maps.Clone(batch.ExpectedVersions)
	if _, _, err := s.resolve(ctx, batch); err != nil {
		return PreparedConfiguration{}, err
	}
	keys := map[string]bool{}
	for _, m := range batch.Manifests {
		keys[Key(m)] = true
	}
	for key, version := range batch.ExpectedVersions {
		if !keys[key] || (version != nil && (len(*version) == 0 || len(*version) > 200)) {
			return PreparedConfiguration{}, ErrInvalid
		}
	}
	if batch.ExpectedVersions == nil {
		batch.ExpectedVersions = map[string]*string{}
	}
	for _, m := range batch.Manifests {
		key := Key(m)
		if _, provided := batch.ExpectedVersions[key]; provided {
			continue
		}
		current, found, err := s.Repo.ResourceKind(ctx, m.Kind, m.Metadata.Name)
		if err != nil {
			return PreparedConfiguration{}, err
		}
		batch.ExpectedVersions[key] = nil
		if found {
			version := current.ResourceVersion
			batch.ExpectedVersions[key] = &version
		}
	}
	if batch.RequestID == "" {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return PreparedConfiguration{}, err
		}
		batch.RequestID = hex.EncodeToString(random[:])
	}
	data, err := json.Marshal(batch)
	if err != nil {
		return PreparedConfiguration{}, err
	}
	var pretty bytes.Buffer
	if err = json.Indent(&pretty, canonicalJSON(data), "", "  "); err != nil {
		return PreparedConfiguration{}, err
	}
	data = pretty.Bytes()
	if len(data) > 1<<20 {
		return PreparedConfiguration{}, ErrInvalid
	}
	return PreparedConfiguration{Source: string(data), RequestID: batch.RequestID, ResourceCount: len(batch.Manifests)}, nil
}
