package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/resources"
)

// SeedCatalog creates missing resources atomically. Persisted resources win over
// bootstrap files, and bootstrap tombstones prevent deleted resources returning.
func (s *Store) SeedCatalog(ctx context.Context, manifests []resources.Manifest, base resources.Registry) error {
	if len(manifests) < 1 || len(manifests) > 20 {
		return fmt.Errorf("bootstrap requires 1..20 resources")
	}
	inputs := map[string]resources.Manifest{}
	for _, m := range manifests {
		key := resources.Key(m)
		if _, exists := inputs[key]; exists {
			return fmt.Errorf("duplicate bootstrap resource %s", key)
		}
		inputs[key] = m
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return err
	}
	current, err := readCatalog(ctx, tx)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, m := range current {
		existing[resources.Key(m)] = true
	}
	keys := make([]string, 0, len(inputs))
	for key := range inputs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	missing := []resources.Manifest{}
	for _, key := range keys {
		m := inputs[key]
		if existing[key] {
			continue
		}
		var seeded bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_resource_bootstrap WHERE kind=$1 AND name=$2)`, m.Kind, m.Metadata.Name).Scan(&seeded); err != nil {
			return err
		}
		if !seeded {
			missing = append(missing, m)
		}
	}
	// Use the same complete resolver as validate/plan/apply, with persisted cursors.
	base.ResolvedPlacements, err = readResolvedPlacements(ctx, tx)
	if err != nil {
		return err
	}
	_, compiled, err := resources.ResolveCatalog(base, current, missing)
	if err != nil {
		return fmt.Errorf("bootstrap catalog: %w", err)
	}
	if base.Active != "" && compiled["Monitor/"+base.Active].Monitor == nil {
		return fmt.Errorf("bootstrap active Monitor is absent or was deleted")
	}
	if err = s.preflightWebhooks(compiled); err != nil {
		return err
	}
	for _, m := range missing {
		uid, e := eventID()
		if e != nil {
			return e
		}
		data, e := json.Marshal(m)
		if e != nil {
			return e
		}
		v, e := scanResource(tx.QueryRow(ctx, `INSERT INTO nmo_resources(name,uid,manifest,effective_revision,kind) VALUES($1,$2,$3,$4,$5) RETURNING `+resourceColumns, m.Metadata.Name, uid, data, compiled[resources.Key(m)].Revision, m.Kind))
		if e != nil {
			return e
		}
		if err = recordResource(ctx, tx, v, "controller-bootstrap"); err != nil {
			return err
		}
	}
	for _, key := range keys {
		m := inputs[key]
		if _, err = tx.Exec(ctx, `INSERT INTO nmo_resource_bootstrap(kind,name) VALUES($1,$2) ON CONFLICT DO NOTHING`, m.Kind, m.Metadata.Name); err != nil {
			return err
		}
	}
	if _, err = refreshDependents(ctx, tx, compiled, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
