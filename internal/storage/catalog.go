package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
)

type catalogReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readCatalog(ctx context.Context, reader catalogReader) ([]resources.Manifest, error) {
	rows, err := reader.Query(ctx, `SELECT manifest FROM nmo_resources WHERE kind IN ('Monitor','DiagnosticProfile','AlertRule','WebhookEndpoint','Target','CheckProfile','AssessmentPolicy','PlacementPolicy','ResourceGroup','Dependency') ORDER BY kind,name`)
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
	base.ResolvedPlacements, err = readResolvedPlacements(ctx, tx)
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
		if entry.Monitor == nil && entry.Group == nil {
			continue
		}
		parts := strings.SplitN(key, "/", 2)
		kind, name := parts[0], parts[1]
		current, err := scanResource(tx.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind=$2 AND name=$1 FOR UPDATE`, name, kind))
		if err != nil {
			return nil, err
		}
		if current.Revision == entry.Revision {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE nmo_resources SET effective_revision=$2 WHERE kind=$3 AND name=$1`, name, entry.Revision, kind); err != nil {
			return nil, err
		}
		if !explicit[key] {
			affected = append(affected, resources.ApplyTarget{Kind: kind, Name: name, UID: current.UID, ResourceVersion: current.ResourceVersion, EffectiveRevision: entry.Revision, Reason: "awaiting_reconciliation", Implicit: true})
		}
	}
	sort.Slice(affected, func(i, j int) bool {
		if affected[i].Name == affected[j].Name {
			return affected[i].Kind < affected[j].Kind
		}
		return affected[i].Name < affected[j].Name
	})
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

// LoadRuntime captures all Monitor plans and shared dependencies in one catalog transaction.
func (s *Store) LoadRuntime(ctx context.Context, base resources.Registry) (resources.Registry, map[string]resources.Compiled, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return base, nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return base, nil, err
	}
	registry, compiled, err := resolveCatalogTx(ctx, tx, base, nil)
	if err != nil {
		return base, nil, err
	}
	// Compiler upgrades may canonicalize an existing profile differently. Refresh
	// its derived hash in the same transaction as dependent Monitor revisions;
	// retain source versions and historical sessions, which describe older code.
	keys := make([]string, 0, len(compiled))
	for key := range compiled {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := compiled[key]
		if entry.Profile == nil {
			continue
		}
		if _, err = tx.Exec(ctx, `UPDATE nmo_resources SET effective_revision=$2 WHERE kind='DiagnosticProfile' AND name=$1 AND effective_revision<>$2`, entry.Profile.Name, entry.Revision); err != nil {
			return base, nil, err
		}
	}
	if _, err = refreshDependents(ctx, tx, compiled, nil); err != nil {
		return base, nil, err
	}
	return registry, compiled, tx.Commit(ctx)
}
func (s *Store) RuntimeCurrent(ctx context.Context, revisions map[string]string) (bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT name,effective_revision FROM nmo_resources WHERE kind='Monitor'`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	count := 0
	current := true
	for rows.Next() {
		var name, revision string
		if err = rows.Scan(&name, &revision); err != nil {
			return false, err
		}
		count++
		if revisions[name] != revision {
			current = false
		}
	}
	return current && count == len(revisions), rows.Err()
}

func readResolvedPlacements(ctx context.Context, reader catalogReader) (map[string]monitor.ResolvedPlacement, error) {
	rows, err := reader.Query(ctx, `SELECT r.name,p.policy_hash,p.plan_hash,p.selected FROM nmo_regular_placements p JOIN nmo_resources r ON r.uid=p.monitor_uid AND r.kind='Monitor'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]monitor.ResolvedPlacement{}
	for rows.Next() {
		var name string
		var value monitor.ResolvedPlacement
		var raw []byte
		if err = rows.Scan(&name, &value.PolicyHash, &value.TemplateHash, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(raw, &value.Selected); err != nil {
			return nil, err
		}
		out[name] = value
	}
	return out, rows.Err()
}
