package storage

import (
	"context"
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/dashboard"
	"github.com/boby-star/NetMonOrc/internal/diagnosticview"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func (s *Store) Dashboard(ctx context.Context, name, revision string, reads dashboard.Reads) (dashboard.Source, error) {
	var out dashboard.Source
	if reads.Resources {
		var raw []byte
		// Count and active-revision evidence come from the same SQL snapshot.
		err := s.pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM nmo_resources WHERE kind='Monitor'),
   (SELECT a.payload FROM nmo_assessments a JOIN nmo_resources r ON r.kind='Monitor' AND r.name=a.monitor AND r.effective_revision=a.revision WHERE a.monitor=$1 AND a.revision=$2)`, name, revision).Scan(&out.Monitors, &raw)
		if err != nil {
			return out, err
		}
		if len(raw) > 0 {
			var a monitor.Assessment
			if err = json.Unmarshal(raw, &a); err != nil {
				return out, err
			}
			out.Assessment = &a
		}
	}
	if reads.Alerts {
		if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM nmo_alerts WHERE api_view->>'state'='firing'`).Scan(&out.Firing); err != nil {
			return out, err
		}
	}
	if reads.Diagnostics {
		rows, err := s.DiagnosticSessions(ctx, diagnosticview.Query{Limit: 5})
		if err != nil {
			return out, err
		}
		if len(rows) > 5 {
			rows = rows[:5]
		}
		out.Diagnostics = rows
	}
	return out, nil
}
