package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
)

const resourceColumns = "uid,version,generation,manifest,effective_revision,created_at,updated_at"

func scanResource(row pgx.Row) (resources.View, error) {
	var v resources.View
	var version int64
	var data []byte
	err := row.Scan(&v.UID, &version, &v.Generation, &data, &v.Revision, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, err
	}
	if err = json.Unmarshal(data, &v.Manifest); err != nil {
		return v, err
	}
	v.ResourceVersion = v.UID + ":" + strconv.FormatInt(version, 10)
	return v, nil
}
func (s *Store) Resource(ctx context.Context, name string) (resources.View, bool, error) {
	return s.ResourceKind(ctx, "Monitor", name)
}
func (s *Store) ResourceKind(ctx context.Context, kind, name string) (resources.View, bool, error) {
	v, err := scanResource(s.pool.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind=$2 AND name=$1`, name, kind))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, err
}
func (s *Store) Resources(ctx context.Context, before, selector string, limit int) ([]resources.View, error) {
	return s.ResourcesKind(ctx, "Monitor", before, selector, limit)
}
func (s *Store) ResourcesKind(ctx context.Context, kind, before, selector string, limit int) ([]resources.View, error) {
	var labels map[string]string
	if selector != "" {
		labels = map[string]string{}
		for _, pair := range strings.Split(selector, ",") {
			key, value, ok := strings.Cut(pair, "=")
			if !ok || key == "" {
				return nil, resources.ErrInvalid
			}
			if _, exists := labels[key]; exists {
				return nil, resources.ErrInvalid
			}
			labels[key] = value
		}
	}
	encoded, _ := json.Marshal(labels)
	if labels == nil {
		encoded = []byte(`{}`)
	}
	rows, err := s.pool.Query(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind=$4 AND ($1='' OR name>$1) AND COALESCE(manifest->'metadata'->'labels','{}'::jsonb) @> $2::jsonb ORDER BY name ASC LIMIT $3`, before, encoded, limit+1, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []resources.View{}
	for rows.Next() {
		v, e := scanResource(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func recordResource(ctx context.Context, tx pgx.Tx, v resources.View, actor string) error {
	version, _ := strconv.ParseInt(strings.SplitN(v.ResourceVersion, ":", 2)[1], 10, 64)
	data, err := json.Marshal(v.Manifest)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO nmo_resource_revisions(uid,version,generation,manifest,actor) VALUES($1,$2,$3,$4,$5)`, v.UID, version, v.Generation, data, actor)
	return err
}
func (s *Store) SeedResource(ctx context.Context, m resources.Manifest, revision string, registry ...resources.Registry) (resources.View, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return resources.View{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return resources.View{}, err
	}
	current, err := scanResource(tx.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind=$2 AND name=$1`, m.Metadata.Name, m.Kind))
	if err == nil {
		if _, err = tx.Exec(ctx, `INSERT INTO nmo_resource_bootstrap(kind,name) VALUES($1,$2) ON CONFLICT DO NOTHING`, m.Kind, m.Metadata.Name); err != nil {
			return current, err
		}
		return current, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return current, err
	}
	var seeded bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_resource_bootstrap WHERE kind=$1 AND name=$2)`, m.Kind, m.Metadata.Name).Scan(&seeded); err != nil {
		return current, err
	}
	if seeded {
		return current, resources.ErrNotFound
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_resource_bootstrap(kind,name) VALUES($1,$2) ON CONFLICT DO NOTHING`, m.Kind, m.Metadata.Name); err != nil {
		return current, err
	}
	if len(registry) > 0 {
		_, compiled, e := resolveCatalogTx(ctx, tx, registry[0], []resources.Manifest{m})
		if e != nil {
			return current, e
		}
		revision = compiled[resources.Key(m)].Revision
	}
	uid, err := eventID()
	if err != nil {
		return current, err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return current, err
	}
	v, err := scanResource(tx.QueryRow(ctx, `INSERT INTO nmo_resources(name,uid,manifest,effective_revision,kind) VALUES($1,$2,$3,$4,$5) RETURNING `+resourceColumns, m.Metadata.Name, uid, data, revision, m.Kind))
	if err != nil {
		return v, err
	}
	if err = recordResource(ctx, tx, v, "controller-bootstrap"); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
func (s *Store) SaveResource(ctx context.Context, actor auth.AdminCredential, m resources.Manifest, registry resources.Registry, expected *string) (resources.View, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return resources.View{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return resources.View{}, err
	}
	username, err := checkAdminScope(ctx, tx, actor, "resources:write")
	if err != nil {
		return resources.View{}, err
	}
	current, err := scanResource(tx.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind=$2 AND name=$1 FOR UPDATE`, m.Metadata.Name, m.Kind))
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return current, err
	}
	if expected == nil && found {
		return current, resources.ErrConflict
	}
	if expected != nil && !found {
		return current, resources.ErrNotFound
	}
	if expected != nil && current.ResourceVersion != *expected {
		return current, resources.ErrVersion
	}
	_, compiled, err := resolveCatalogTx(ctx, tx, registry, []resources.Manifest{m})
	if err != nil {
		return current, err
	}
	v, err := saveResourceTx(ctx, tx, m, compiled[resources.Key(m)].Revision, current, found, username)
	if err != nil {
		return v, err
	}
	if _, err = refreshDependents(ctx, tx, compiled, map[string]bool{resources.Key(m): true}); err != nil {
		return v, err
	}
	return v, tx.Commit(ctx)
}
func (s *Store) DeleteResource(ctx context.Context, actor auth.AdminCredential, name, expected string) error {
	return s.DeleteResourceKind(ctx, actor, "Monitor", name, expected, resources.Registry{})
}
func (s *Store) DeleteResourceKind(ctx context.Context, actor auth.AdminCredential, kind, name, expected string, registry resources.Registry) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return err
	}
	username, err := checkAdminScope(ctx, tx, actor, "resources:write")
	if err != nil {
		return err
	}
	v, err := scanResource(tx.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind=$2 AND name=$1 FOR UPDATE`, name, kind))
	if errors.Is(err, pgx.ErrNoRows) {
		return resources.ErrNotFound
	}
	if err != nil {
		return err
	}
	if v.ResourceVersion != expected {
		return resources.ErrVersion
	}
	if kind == "DiagnosticProfile" {
		var refs bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_resources WHERE kind='Monitor' AND manifest->'spec'->'diagnostics'->>'profileRef'=$1 AND COALESCE((manifest->'spec'->'diagnostics'->>'enabled')::boolean,false))`, name).Scan(&refs); err != nil {
			return err
		}
		if refs {
			return resources.ErrConflict
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_resource_bootstrap(kind,name) VALUES($1,$2) ON CONFLICT DO NOTHING`, kind, name); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM nmo_resources WHERE kind=$2 AND name=$1`, name, kind); err != nil {
		return err
	}
	_, compiled, e := resolveCatalogTx(ctx, tx, registry, nil)
	if e != nil {
		return e
	}
	if _, e = refreshDependents(ctx, tx, compiled, nil); e != nil {
		return e
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, username, "resource.delete:"+kind+"/"+name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) ResourceHistory(ctx context.Context, name, before string, limit int) ([]resources.Revision, error) {
	return s.ResourceHistoryKind(ctx, "Monitor", name, before, limit)
}
func (s *Store) ResourceHistoryKind(ctx context.Context, kind, name, before string, limit int) ([]resources.Revision, error) {
	current, found, err := s.ResourceKind(ctx, kind, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, resources.ErrNotFound
	}
	maxVersion := int64(9223372036854775807)
	if before != "" {
		if !strings.HasPrefix(before, current.UID+":") {
			return nil, resources.ErrInvalid
		}
		maxVersion, err = strconv.ParseInt(strings.TrimPrefix(before, current.UID+":"), 10, 64)
		if err != nil || maxVersion < 1 {
			return nil, resources.ErrInvalid
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT uid,version,generation,manifest,changed_at,actor FROM nmo_resource_revisions WHERE uid=$1 AND version<$2 ORDER BY version DESC LIMIT $3`, current.UID, maxVersion, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []resources.Revision{}
	for rows.Next() {
		var v resources.Revision
		var version int64
		var data []byte
		if err = rows.Scan(&v.UID, &version, &v.Generation, &data, &v.ChangedAt, &v.Actor); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &v.Manifest); err != nil {
			return nil, err
		}
		v.ResourceVersion = fmt.Sprintf("%s:%d", v.UID, version)
		result = append(result, v)
	}
	return result, rows.Err()
}

// ReadLiveAssessment gives the API one consistent database snapshot: a result
// belongs to both the desired resource revision and the loaded Controller plan.
func (s *Store) ReadLiveAssessment(ctx context.Context, name, revision string) (monitor.Assessment, bool, error) {
	var a monitor.Assessment
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT a.payload FROM nmo_assessments a JOIN nmo_resources r ON r.name=a.monitor AND r.kind='Monitor' WHERE a.monitor=$1 AND a.revision=$2 AND r.effective_revision=$2`, name, revision).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, false, nil
	}
	if err != nil {
		return a, false, err
	}
	if err = json.Unmarshal(data, &a); err != nil {
		return a, false, err
	}
	return a, true, nil
}

func saveResourceTx(ctx context.Context, tx pgx.Tx, m resources.Manifest, revision string, current resources.View, found bool, username string) (resources.View, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return resources.View{}, err
	}
	var v resources.View
	if found {
		v, err = scanResource(tx.QueryRow(ctx, `UPDATE nmo_resources SET generation=generation+CASE WHEN manifest->'spec' IS DISTINCT FROM $2::jsonb->'spec' THEN 1 ELSE 0 END,version=version+1,manifest=$2,effective_revision=$3,updated_at=now() WHERE kind=$4 AND name=$1 RETURNING `+resourceColumns, m.Metadata.Name, data, revision, m.Kind))
	} else {
		uid, e := eventID()
		if e != nil {
			return v, e
		}
		v, err = scanResource(tx.QueryRow(ctx, `INSERT INTO nmo_resources(name,uid,manifest,effective_revision,kind) VALUES($1,$2,$3,$4,$5) RETURNING `+resourceColumns, m.Metadata.Name, uid, data, revision, m.Kind))
	}
	if err != nil {
		return v, err
	}
	if err = recordResource(ctx, tx, v, username); err != nil {
		return v, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO nmo_identity_audit(username,action,succeeded) VALUES($1,$2,true)`, username, "resource.save:"+m.Kind+"/"+m.Metadata.Name); err != nil {
		return v, err
	}
	return v, nil
}
