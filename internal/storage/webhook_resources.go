package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/boby-star/NetMonOrc/internal/webhook"
	"github.com/jackc/pgx/v5"
)

// ConfigureManagedWebhooks is called before serving requests. The infrastructure
// preflight checks local TLS/secrets without putting I/O inside the compiler.
func (s *Store) ConfigureManagedWebhooks(preflight func(webhook.Endpoint) error) {
	s.managedWebhooks = true
	s.webhookPreflight = preflight
}
func (s *Store) preflightWebhooks(compiled map[string]resources.Compiled) error {
	for _, entry := range compiled {
		if entry.Endpoint == nil || !entry.Endpoint.Enabled {
			continue
		}
		if s.webhookPreflight == nil {
			return fmt.Errorf("%w: webhook runtime is unavailable", resources.ErrInvalid)
		}
		if err := s.webhookPreflight(*entry.Endpoint); err != nil {
			return fmt.Errorf("%w: webhook local credentials/TLS configuration unavailable", resources.ErrInvalid)
		}
	}
	return nil
}
func (s *Store) WebhookResources(ctx context.Context) ([]resources.View, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind='WebhookEndpoint' ORDER BY name LIMIT $1`, resources.MaxCatalogResources+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []resources.View{}
	for rows.Next() {
		v, err := scanResource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		if len(out) > resources.MaxCatalogResources {
			return nil, resources.ErrInvalid
		}
	}
	return out, rows.Err()
}

// WebhookFingerprint includes identity and normalized configuration, but not
// labels or resourceVersion: metadata changes do not require a sender reload.
func WebhookFingerprint(rows []resources.View) string {
	keys := make([]string, 0, len(rows))
	for _, v := range rows {
		keys = append(keys, v.Manifest.Metadata.Name+"/"+v.UID+"/"+v.Revision)
	}
	sort.Strings(keys)
	b, _ := json.Marshal(keys)
	return string(b)
}
func (s *Store) webhookCurrent(ctx context.Context, tx pgx.Tx, endpoint webhook.Endpoint) (bool, error) {
	if !s.managedWebhooks {
		return true, nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock_shared(781305)`); err != nil {
		return false, err
	}
	if endpoint.Name == "" {
		var empty bool
		err := tx.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM nmo_resources WHERE kind='WebhookEndpoint')`).Scan(&empty)
		return empty && !endpoint.Enabled, err
	}
	var hash string
	err := tx.QueryRow(ctx, `SELECT effective_revision FROM nmo_resources WHERE kind='WebhookEndpoint' AND name=$1 FOR SHARE`, endpoint.Name).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return hash == endpoint.Hash(), err
}
