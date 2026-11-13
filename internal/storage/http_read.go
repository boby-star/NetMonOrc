package storage

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ReadAssessment(ctx context.Context, name string) (monitor.Assessment, bool, error) {
	var a monitor.Assessment
	var b []byte
	err := s.pool.QueryRow(ctx, "SELECT payload FROM nmo_assessments WHERE monitor=$1", name).Scan(&b)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, false, nil
	}
	if err != nil {
		return a, false, err
	}
	err = json.Unmarshal(b, &a)
	return a, err == nil, err
}
