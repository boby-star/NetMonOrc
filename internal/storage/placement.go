package storage

import (
	"context"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/agentstatus"
	"github.com/boby-star/NetMonOrc/internal/placement"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

// ReconcilePlacement runs under the same desired-catalog lock as apply. Policy
// changes and selections therefore cannot publish incompatible effective hashes.
func (s *Store) ReconcilePlacement(ctx context.Context, base resources.Registry, agents []agentstatus.View, now time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(781305)`); err != nil {
		return false, err
	}
	registry, compiled, err := resolveCatalogTx(ctx, tx, base, nil)
	if err != nil {
		return false, err
	}
	names := []string{}
	for key, value := range compiled {
		if value.Monitor == nil {
			continue
		}
		p := value.Monitor
		if p.FixedPlacement != nil && p.FixedPlacement.Strategy != "" {
			names = append(names, key)
		}
		for id, binding := range p.CheckPlacements {
			if binding.Strategy != "" {
				names = append(names, key+"/"+id)
			}
		}
	}
	sort.Strings(names)
	type previousResource struct{ uid, revision string }
	previousRevisions := map[string]previousResource{}
	rows, err := tx.Query(ctx, `SELECT kind,name,uid,effective_revision FROM nmo_resources WHERE kind IN ('Monitor','ResourceGroup')`)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var kind, name string
		var v previousResource
		if err = rows.Scan(&kind, &name, &v.uid, &v.revision); err != nil {
			rows.Close()
			return false, err
		}
		previousRevisions[kind+"/"+name] = v
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	changed := false
	for _, key := range names {
		parts := strings.SplitN(key, "/", 3)
		parentKey := parts[0] + "/" + parts[1]
		plan := *compiled[parentKey].Monitor
		checkID := ""
		if len(parts) == 3 {
			checkID = parts[2]
		}
		policy := plan.FixedPlacement
		if checkID != "" {
			policy = plan.CheckPlacements[checkID]
		}

		resource, loadErr := scanResource(tx.QueryRow(ctx, `SELECT `+resourceColumns+` FROM nmo_resources WHERE kind='Monitor' AND name=$1`, plan.Name))
		if loadErr != nil {
			return false, loadErr
		}
		uid := resource.UID
		template, templateErr := registry.PlacementTemplate(resource.Manifest)
		if templateErr != nil {
			return false, templateErr
		}

		cursorKey := "Monitor/" + uid
		if checkID != "" {
			cursorKey += "/" + checkID
		}
		if policy.Ref != "" {
			if err = tx.QueryRow(ctx, `SELECT uid FROM nmo_resources WHERE kind='PlacementPolicy' AND name=$1`, policy.Ref).Scan(&cursorKey); err != nil {
				return false, err
			}
			cursorKey = "PlacementPolicy/" + cursorKey
		}
		if _, err = tx.Exec(ctx, `INSERT INTO nmo_placement_cursors(policy_key) VALUES($1) ON CONFLICT DO NOTHING`, cursorKey); err != nil {
			return false, err
		}
		var cursor int64
		if err = tx.QueryRow(ctx, `SELECT next_offset FROM nmo_placement_cursors WHERE policy_key=$1 FOR UPDATE`, cursorKey).Scan(&cursor); err != nil {
			return false, err
		}
		selectionPlan := plan
		selectionPlan.CheckPlacements = nil
		selectionPlan.FixedPlacement = policy
		selectionPlan.Checks = nil
		for _, check := range plan.Checks {
			if (checkID != "" && check.ID == checkID) || (checkID == "" && plan.CheckPlacements[check.ID] == nil) {
				selectionPlan.Checks = append(selectionPlan.Checks, check)
			}
		}
		selected, next := placement.Select(selectionPlan, agents, cursor, now)
		stateKey := plan.Name
		if checkID != "" {
			stateKey += "/" + checkID
		}
		previous, exists := registry.ResolvedPlacements[stateKey]
		if exists && previous.PolicyHash == policy.Hash && previous.TemplateHash == template.Revision && slices.Equal(previous.Selected, selected) {
			continue
		}
		raw, err := json.Marshal(selected)
		if err != nil {
			return false, err
		}
		if checkID == "" {
			_, err = tx.Exec(ctx, `INSERT INTO nmo_regular_placements(monitor_uid,policy_hash,selected,plan_hash) VALUES($1,$2,$3,$4) ON CONFLICT(monitor_uid) DO UPDATE SET policy_hash=EXCLUDED.policy_hash,selected=EXCLUDED.selected,plan_hash=EXCLUDED.plan_hash`, uid, policy.Hash, raw, template.Revision)
		} else {
			_, err = tx.Exec(ctx, `INSERT INTO nmo_check_placements(monitor_uid,check_id,policy_hash,selected,plan_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(monitor_uid,check_id) DO UPDATE SET policy_hash=EXCLUDED.policy_hash,selected=EXCLUDED.selected,plan_hash=EXCLUDED.plan_hash`, uid, checkID, policy.Hash, raw, template.Revision)
		}
		if err != nil {
			return false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE nmo_placement_cursors SET next_offset=$2 WHERE policy_key=$1`, cursorKey, next); err != nil {
			return false, err
		}
		changed = true
	}
	if changed {
		_, compiled, err = resolveCatalogTx(ctx, tx, base, nil)
		if err != nil {
			return false, err
		}
		if _, err = refreshDependents(ctx, tx, compiled, nil); err != nil {
			return false, err
		}
		for key, value := range compiled {
			before, found := previousRevisions[key]
			if !found || before.revision == value.Revision {
				continue
			}
			if _, err = tx.Exec(ctx, `INSERT INTO nmo_placement_transitions(resource_uid,from_revision,to_revision) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, before.uid, before.revision, value.Revision); err != nil {
				return false, err
			}
		}

	}
	return changed, tx.Commit(ctx)
}
