package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func (s *Store) RecordConfigRejection(ctx context.Context, agent string, generation int64, revision string) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var current int64
	if err = tx.QueryRow(ctx, `SELECT generation FROM nmo_sessions WHERE agent_id=$1 FOR UPDATE`, agent).Scan(&current); err != nil {
		return false, err
	}
	if current != generation {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `INSERT INTO nmo_config_rejections(agent_id,revision) SELECT $1,$2 WHERE EXISTS(SELECT 1 FROM nmo_assignments WHERE agent_id=$1 AND revision=$2) ON CONFLICT(agent_id,revision) DO UPDATE SET rejected_at=clock_timestamp()`, agent, revision)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, tx.Commit(ctx)
}
func (s *Store) ConfigRejected(ctx context.Context, agent, revision string) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM nmo_config_rejections WHERE agent_id=$1 AND revision=$2)`, agent, revision).Scan(&exists)
	return exists, err
}

func (s *Store) Latest(ctx context.Context, p monitor.Plan, now time.Time) ([]monitor.Observation, error) {
	regular, err := s.LatestRegular(ctx, p, now)
	if err != nil || len(p.FallbackRoster) == 0 {
		return regular, err
	}
	rows, err := s.pool.Query(ctx, `SELECT o.payload,r.payload FROM nmo_fallback_tasks t JOIN nmo_fallback_runs r ON r.id=t.run_id JOIN nmo_observations o ON o.fallback_task_id=t.id WHERE t.monitor=$1 AND t.revision=$2 AND r.payload->>'strategy'='same-zone' AND o.finished_at>=$3 ORDER BY o.finished_at DESC,o.id DESC LIMIT 10000`, p.Name, p.Revision, now.Add(-p.MaxAge))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	latest := map[string]monitor.Observation{}
	for _, o := range regular {
		latest[o.Agent+"/"+o.Check] = o
	}
	for rows.Next() {
		var ob, rb []byte
		if err = rows.Scan(&ob, &rb); err != nil {
			return nil, err
		}
		var o monitor.Observation
		var r FallbackRun
		if err = json.Unmarshal(ob, &o); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(rb, &r); err != nil {
			return nil, err
		}
		original, exists := latest[r.OriginAgent+"/"+r.Check]
		if r.OriginID != "" && (!exists || original.ID != r.OriginID) {
			continue
		}
		if r.OriginID == "" && exists && !original.FinishedAt.Before(r.StartedAt) {
			continue
		}
		o.FallbackForAgent = r.OriginAgent
		o.FallbackRunID = r.ID
		regular = append(regular, o)
	}
	return regular, rows.Err()
}
