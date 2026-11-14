package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

// ReadObservations reads at most limit+1 rows; historical revisions and diagnostic
// provenance remain visible without being reintroduced into live assessment.
func (s *Store) ReadObservations(ctx context.Context, q monitor.HistoryQuery) ([]monitor.ObservationRecord, error) {
	if q.Limit < 1 || q.Limit > 200 || q.Monitor == "" {
		return nil, fmt.Errorf("invalid bounded observation query")
	}
	rows, err := s.pool.Query(ctx, `SELECT o.payload,o.received_at,o.finished_at,COALESCE(t.run_id,'')
 FROM nmo_observations o LEFT JOIN nmo_diagnostic_tasks t ON t.id=o.diagnostic_task_id
 WHERE o.monitor=$1 AND ($2='' OR o.payload->>'zone'=$2)
 AND ($3::timestamptz IS NULL OR o.finished_at>=$3)
 AND ($4::timestamptz IS NULL OR o.finished_at<=$4)
 AND ($5::timestamptz IS NULL OR (o.finished_at,o.id)<($5,$6))
 ORDER BY o.finished_at DESC,o.id DESC LIMIT $7`, q.Monitor, q.Zone, historyTime(q.Since), historyTime(q.Until), historyTime(q.Before), q.BeforeID, q.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []monitor.ObservationRecord{}
	for rows.Next() {
		var record monitor.ObservationRecord
		var data []byte
		if err = rows.Scan(&data, &record.ReceivedAt, &record.SortFinishedAt, &record.DiagnosticSessionID); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(data, &record.Observation); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func historyTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}
