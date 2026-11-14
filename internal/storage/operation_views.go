package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/boby-star/NetMonOrc/internal/operationview"
	"github.com/jackc/pgx/v5"
)

func scanOperationView(row pgx.Row) (operationview.Operation, error) {
	var v operationview.Operation
	var raw []byte
	err := row.Scan(&v.ID, &v.Kind, &v.State, &v.StartedAt, &v.FinishedAt, &raw)
	if err != nil {
		return v, err
	}
	var payload struct {
		Error  *operationview.Error `json:"error"`
		Result json.RawMessage      `json:"result"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return v, err
	}
	v.Error = payload.Error
	v.Result = payload.Result
	return v, nil
}
func (s *Store) ReadOperation(ctx context.Context, id string) (operationview.Operation, bool, error) {
	v, err := scanOperationView(s.pool.QueryRow(ctx, `SELECT id,kind,state,started_at,finished_at,payload FROM nmo_operations WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, err
}
func (s *Store) ReadOperations(ctx context.Context, before, state string, kinds []string, limit int) ([]operationview.Operation, error) {
	if limit < 1 || limit > 200 {
		return nil, fmt.Errorf("invalid bounded operation query")
	}
	rows, err := s.pool.Query(ctx, `SELECT id,kind,state,started_at,finished_at,payload FROM nmo_operations WHERE kind=ANY($1) AND ($2='' OR id<$2) AND ($3='' OR state=$3) ORDER BY id DESC LIMIT $4`, kinds, before, state, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []operationview.Operation{}
	for rows.Next() {
		v, e := scanOperationView(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
