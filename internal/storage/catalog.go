package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
)

type catalogReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readCatalog(ctx context.Context, reader catalogReader) ([]resources.Manifest, error) {
	rows, err := reader.Query(ctx, `SELECT manifest FROM nmo_resources WHERE kind IN ('Monitor','DiagnosticProfile') ORDER BY kind,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.Manifest{}
	size := 0
	for rows.Next() {
		var data []byte
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		var m resources.Manifest
		if err = json.Unmarshal(data, &m); err != nil {
			return nil, err
		}
		// PostgreSQL JSONB adds whitespace on output. Count the same canonical
		// envelope as admission so an accepted catalog remains readable.
		canonical, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		size += len(canonical)
		if size > resources.MaxCatalogBytes || len(out) >= resources.MaxCatalogResources {
			return nil, fmt.Errorf("%w: catalog exceeds research runtime capacity", resources.ErrInvalid)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
func (s *Store) Catalog(ctx context.Context) ([]resources.Manifest, error) {
	return readCatalog(ctx, s.pool)
}
func resolveCatalogTx(ctx context.Context, tx pgx.Tx, base resources.Registry, updates []resources.Manifest) (resources.Registry, map[string]resources.Compiled, error) {
	current, err := readCatalog(ctx, tx)
	if err != nil {
		return base, nil, err
	}
	registry, compiled, err := resources.ResolveCatalog(base, current, updates)
	if err != nil {
		return base, nil, fmt.Errorf("%w: %s", resources.ErrInvalid, err)
	}
	return registry, compiled, nil
}

// refreshDependents changes only effective revisions; the referenced Monitor's
// own manifest/resourceVersion and immutable revision history remain unchanged.
func refreshDependents(ctx context.Context, tx pgx.Tx, compiled map[string]resources.Compiled, explicit map[string]bool) ([]resources.ApplyTarget, error) {
	affected := []resources.ApplyTarget{}
	for key, entry := range compiled {
		if entry.Monitor == nil {
			continue
		}
		name := entry.Monitor.Name
		current, err := scanResource(tx.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind='Monitor' AND name=$1 FOR UPDATE`, name))
		if err != nil {
			return nil, err
		}
		if current.Revision == entry.Revision {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE nmo_resources SET effective_revision=$2 WHERE kind='Monitor' AND name=$1`, name, entry.Revision); err != nil {
			return nil, err
		}
		if !explicit[key] {
			affected = append(affected, resources.ApplyTarget{Kind: "Monitor", Name: name, UID: current.UID, ResourceVersion: current.ResourceVersion, EffectiveRevision: entry.Revision, Reason: "awaiting_reconciliation", Implicit: true})
		}
	}
	sort.Slice(affected, func(i, j int) bool { return affected[i].Name < affected[j].Name })
	return affected, nil
}

// LoadCompiledMonitor captures the profile catalog and active IR under the same
// writer lock used by apply, avoiding a stale dependency hash during startup.
func (s *Store) LoadCompiledMonitor(ctx context.Context, name string, base resources.Registry) (resources.Registry, resources.Compiled, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return base, resources.Compiled{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return base, resources.Compiled{}, err
	}
	registry, compiled, err := resolveCatalogTx(ctx, tx, base, nil)
	if err != nil {
		return base, resources.Compiled{}, err
	}
	plan, found := compiled["Monitor/"+name]
	if !found {
		return base, plan, resources.ErrNotFound
	}
	if _, err = refreshDependents(ctx, tx, compiled, nil); err != nil {
		return base, plan, err
	}
	return registry, plan, tx.Commit(ctx)
}
