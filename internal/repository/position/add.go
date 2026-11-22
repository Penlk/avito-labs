package position

import (
	"context"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domain "github.com/Penlk/avito-labs/internal/domain/position"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
)

func (r *Repository) Add(ctx context.Context, pos domain.Position) (domain.Position, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	query, args, err := squirrel.Insert("trip_positions").
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(pos.TripId(), pos.Coordinates().Latitude(), pos.Coordinates().Longitude(), pos.RecordedAt()).
		PlaceholderFormat(squirrel.Dollar).
		Suffix("RETURNING " + positionColumns).
		ToSql()
	if err != nil {
		return domain.Position{}, fmt.Errorf("build insert position query: %w", err)
	}

	rows, err := transaction.Executor(ctx, r.pool).Query(ctx, query, args...)
	if err != nil {
		return domain.Position{}, fmt.Errorf("insert position: %w", classifyPositionWriteError(err))
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[PositionRow])
	if err != nil {
		return domain.Position{}, fmt.Errorf("read inserted position: %w", classifyPositionWriteError(err))
	}

	result, err := row.ToPosition()
	if err != nil {
		return domain.Position{}, fmt.Errorf("decode inserted position: %w", err)
	}
	return result, nil
}
