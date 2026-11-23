package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
	"time"
)

func alertBinding(m resources.Manifest) ([]byte, error) {
	m.Metadata = resources.Metadata{Name: m.Metadata.Name}
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	rule, err := config.CompileAlert(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal([]any{rule.Monitor, rule.Scope, rule.Selector})
}
func protectAlertBinding(ctx context.Context, tx pgx.Tx, current, next resources.Manifest) error {
	before, err := alertBinding(current)
	if err != nil {
		return err
	}
	after, err := alertBinding(next)
	if err != nil {
		return err
	}
	if bytes.Equal(before, after) {
		return nil
	}
	var active bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_alerts WHERE logical_key LIKE $1 AND payload->>'status' IN ('pending','firing'))`, current.Metadata.Name+"/%").Scan(&active); err != nil {
		return err
	}
	if active {
		return resources.ErrConflict
	}
	return nil
}

// Readiness proves every currently selected Monitor/zone at its current revision.
// A selector matching no catalog resources is stored, not evaluated.
func alertRuleReady(ctx context.Context, tx pgx.Tx, view resources.View, runtimes map[string]RuntimeProof, since time.Time) (bool, bool, error) {
	m := view.Manifest
	m.Metadata = resources.Metadata{Name: m.Metadata.Name}
	data, err := json.Marshal(m)
	if err != nil {
		return false, false, err
	}
	rule, err := config.CompileAlert(data)
	if err != nil {
		return false, false, err
	}
	rows, err := tx.Query(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind='Monitor' ORDER BY name`)
	if err != nil {
		return false, false, err
	}
	monitors := []resources.View{}
	for rows.Next() {
		v, err := scanResource(rows)
		if err != nil {
			rows.Close()
			return false, false, err
		}
		if rule.Matches(v.Manifest.Metadata.Name, v.Manifest.Metadata.Labels) {
			monitors = append(monitors, v)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, false, err
	}
	if len(monitors) == 0 {
		return rule.Selector != nil, rule.Selector != nil, nil
	}
	for _, mon := range monitors {
		name := mon.Manifest.Metadata.Name
		runtime, ok := runtimes[name]
		if !ok || runtime.Revision != mon.Revision {
			return false, false, nil
		}
		keys := []string{rule.Name + "/" + name + "/monitor"}
		if rule.Scope == "zone" {
			keys = nil
			for _, zone := range runtime.Zones {
				keys = append(keys, rule.Name+"/"+name+"/zone/"+zone)
			}
			if len(keys) == 0 {
				return false, false, nil
			}
		}
		for _, key := range keys {
			var applied bool
			err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_alerts WHERE logical_key=$1 AND payload->>'ruleHash'=$2 AND (payload->>'lastEvaluatedAt')::timestamptz >= $3 AND payload->>'revision'=$4)`, key, view.Revision, since, runtime.Revision).Scan(&applied)
			if err != nil {
				return false, false, err
			}
			if !applied {
				return false, false, nil
			}
		}
	}
	return true, false, nil
}
