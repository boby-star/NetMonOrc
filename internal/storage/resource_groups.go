package storage

import (
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"github.com/boby-star/NetMonOrc/internal/resources"
	"github.com/jackc/pgx/v5"
	"time"
)

// Group membership, desired revisions and evidence share a repeatable-read
// snapshot. A result from before a Monitor's identity was created cannot revive it.
func (s *Store) ReadGroupAssessment(ctx context.Context, name string, base resources.Registry) (resourcegroup.Assessment, bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return resourcegroup.Assessment{}, false, err
	}
	defer tx.Rollback(ctx)
	catalog, err := readCatalog(ctx, tx)
	if err != nil {
		return resourcegroup.Assessment{}, false, err
	}
	found := false
	for _, m := range catalog {
		found = found || (m.Kind == "ResourceGroup" && m.Metadata.Name == name)
	}
	if !found {
		return resourcegroup.Assessment{}, false, nil
	}
	_, compiled, err := resources.ResolveCatalog(base, catalog, nil)
	if err != nil {
		return resourcegroup.Assessment{}, false, err
	}
	g := compiled["ResourceGroup/"+name]
	sources := map[string]resourcegroup.Source{}
	rows, err := tx.Query(ctx, `SELECT r.name,r.effective_revision,a.payload FROM nmo_resources r LEFT JOIN nmo_assessments a ON a.monitor=r.name AND a.revision=r.effective_revision AND a.evaluated_at>=r.created_at WHERE r.kind='Monitor' AND r.name=ANY($1) ORDER BY r.name`, g.GroupMembers)
	if err != nil {
		return resourcegroup.Assessment{}, false, err
	}
	for rows.Next() {
		var id, rev string
		var raw []byte
		if err = rows.Scan(&id, &rev, &raw); err != nil {
			rows.Close()
			return resourcegroup.Assessment{}, false, err
		}
		source := resourcegroup.Source{Member: resourcegroup.Member{MonitorID: id, State: "unknown", CoverageState: "unknown", Revision: rev}}
		if len(raw) > 0 {
			var a monitor.Assessment
			if err = json.Unmarshal(raw, &a); err != nil {
				rows.Close()
				return resourcegroup.Assessment{}, false, err
			}
			if a.Monitor == id && a.Revision == rev {
				source.At = a.At
				source.Member.State = a.State
				source.Member.CoverageState = "sufficient"
				for _, z := range a.Zones {
					if z.Coverage != "complete" {
						source.Member.CoverageState = "degraded"
					}
					source.EvidenceIDs = append(source.EvidenceIDs, z.Evidence...)
				}
				if len(source.EvidenceIDs) == 0 {
					source.Member.CoverageState = "unknown"
				}
			}
		}
		sources[id] = source
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return resourcegroup.Assessment{}, false, err
	}
	result := resourcegroup.Evaluate(name, *g.Group, g.GroupMembers, sources, time.Now().UTC())
	return result, true, tx.Commit(ctx)
}
