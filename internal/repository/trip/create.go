package trip

import (
	"context"
	"errors"
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
)

func (r *Repository) Create(ctx context.Context, trip domain.Trip) (domain.Trip, error) {
	if trip.State() == nil {
		return domain.Trip{}, errors.New("insert trip: missing status")
	}
	executor := transaction.Executor(ctx, r.pool)
	price := trip.Price()

	query, args, err := squirrel.Insert("trips").
		SetMap(map[string]any{
			"id":              trip.Id(),
			"user_id":         trip.UserId(),
			"driver_id":       trip.DriverId(),
			"start_latitude":  trip.StartPoint().Latitude(),
			"start_longitude": trip.StartPoint().Longitude(),
			"end_latitude":    trip.EndPoint().Latitude(),
			"end_longitude":   trip.EndPoint().Longitude(),
			"price":           price.Rubles(),
			statusColumn:      trip.State().String(),
			"started_at":      trip.StartedAt(),
			"finished_at":     trip.FinishedAt(),
		}).
		PlaceholderFormat(squirrel.Dollar).
		Suffix("RETURNING " + tripColumns).
		ToSql()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("build insert trip query: %w", err)
	}

	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("insert trip: %w", classifyTripWriteError(err))
	}

	tripRow, err := pgx.CollectExactlyOneRow(
		rows, pgx.RowToStructByName[TripRow],
	)
	if err != nil {
		return domain.Trip{}, fmt.Errorf("read inserted trip: %w", classifyTripWriteError(err))
	}

	createdTrip, err := tripRow.ToTrip()
	if err != nil {
		return domain.Trip{}, fmt.Errorf("decode inserted trip: %w", err)
	}
	return createdTrip, nil
}
