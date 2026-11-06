package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
	"github.com/Penlk/avito-labs/internal/usecase/trip/finish"
)

func (r *Repository) UpdateState(ctx context.Context, trip domain.Trip, from domain.TripStatusState) (domain.Trip, error) {
	if trip.State() == nil {
		return domain.Trip{}, errors.New("update trip: missing status")
	}
	executor := transaction.Executor(ctx, r.pool)

	status := trip.State().String()

	query, args, err := squirrel.Update("trips").
		SetMap(map[string]any{
			statusColumn:  status,
			"finished_at": trip.FinishedAt(),
			"updated_at":  squirrel.Expr("now()"),
		}).
		Where(squirrel.Eq{"id": trip.Id(), statusColumn: from.String()}).
		PlaceholderFormat(squirrel.Dollar).
		Suffix("RETURNING " + tripColumns).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build update trip query: %w", err)
	}
	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("update trip: %w", classifyTripWriteError(err))
	}

	tripRow, err := pgx.CollectExactlyOneRow(
		rows, pgx.RowToStructByName[TripRow],
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Trip{}, fmt.Errorf("update trip: %w: %w", finish.ErrTripNotUpdated, err)
	}
	if err != nil {
		return domain.Trip{}, fmt.Errorf("read updated trip: %w", classifyTripWriteError(err))
	}

	updatedTrip, err := tripRow.ToTrip()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("decode updated trip: %w", err)
	}
	return updatedTrip, nil
}
