package storage

import (
	"context"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func (s *Store) AlertRuleResources(ctx context.Context, monitor string) ([]resources.View, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind='AlertRule' AND (manifest->'spec'->>'monitorRef'=$1 OR manifest->'spec' ? 'selector') ORDER BY name LIMIT $2`, monitor, resources.MaxCatalogResources+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []resources.View{}
	for rows.Next() {
		v, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
		if len(result) > resources.MaxCatalogResources {
			return nil, resources.ErrInvalid
		}
	}
	return result, rows.Err()
}
