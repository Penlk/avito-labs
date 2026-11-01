package trips

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/Penlk/avito-labs/internal/postgres"
	"github.com/Penlk/avito-labs/internal/trips/vo"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oapi-codegen/runtime/types"
)

const tripColumns = `id, user_id, driver_id,
	start_latitude, start_longitude, end_latitude, end_longitude,
	price, status, started_at, finished_at`

type Repository interface {
	Create(ctx context.Context, trip Trip) (Trip, error)
	GetById(ctx context.Context, id types.UUID) (Trip, error)
	UpdateState(ctx context.Context, trip Trip, from TripStatusState) (Trip, error)
	AddStatusHistory(ctx context.Context, id types.UUID, from *string, to string) error
}

type repositoryImpl struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

type TripRow struct {
	DriverId types.UUID `db:"driver_id"`

	End_latitude   float64    `db:"end_latitude"`
	End_longitude  float64    `db:"end_longitude"`
	FinishedAt     *time.Time `db:"finished_at"`
	Id             types.UUID `db:"id"`
	LastPositionAt *time.Time `db:"-"`

	Price int64 `db:"price"`

	Start_latitude  float64    `db:"start_latitude"`
	Start_longitude float64    `db:"start_longitude"`
	StartedAt       time.Time  `db:"started_at"`
	Status          string     `db:"status"`
	UserId          types.UUID `db:"user_id"`
}

func (t *TripRow) ToTrip() (Trip, error) {
	var state TripStatusCore
	switch t.Status {
	case "active":
		state.state = ActiveState{}
	case "completed":
		state.state = CompletedState{}
	default:
		return Trip{}, fmt.Errorf("unknown status: %s", t.Status)
	}

	startPoint, err := vo.NewCoordinates(t.Start_latitude, t.Start_longitude)
	if err != nil {
		return Trip{}, err
	}

	endPoint, err := vo.NewCoordinates(t.End_latitude, t.End_longitude)
	if err != nil {
		return Trip{}, err
	}

	price, err := vo.NewPrice(t.Price)
	if err != nil {
		return Trip{}, err
	}

	return *RestoreTrip(
		t.Id,
		t.DriverId,
		t.UserId,
		endPoint,
		t.FinishedAt,
		t.LastPositionAt,
		price,
		startPoint,
		t.StartedAt,
		state,
	), nil
}

func (r *repositoryImpl) Create(ctx context.Context, trip Trip) (Trip, error) {
	if trip.status.State() == nil {
		return Trip{}, errors.New("insert trip: missing status")
	}
	executor := postgres.Executor(ctx, r.pool)

	query, args, err := squirrel.Insert("trips").
		SetMap(map[string]any{
			"id":              trip.id,
			"user_id":         trip.userId,
			"driver_id":       trip.driverId,
			"start_latitude":  trip.startPoint.Latitude(),
			"start_longitude": trip.startPoint.Longitude(),
			"end_latitude":    trip.endPoint.Latitude(),
			"end_longitude":   trip.endPoint.Longitude(),
			"price":           trip.price.Rubles(),
			"status":          trip.status.State().String(),
			"started_at":      trip.startedAt,
			"finished_at":     trip.finishedAt,
		}).
		PlaceholderFormat(squirrel.Dollar).
		Suffix("RETURNING " + tripColumns).
		ToSql()

	if err != nil {
		return Trip{}, fmt.Errorf("build insert trip query: %w", err)
	}

	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return Trip{}, fmt.Errorf("insert trip: %w", classifyTripWriteError(err))
	}

	tripRow, err := pgx.CollectExactlyOneRow(
		rows, pgx.RowToStructByName[TripRow],
	)
	if err != nil {
		return Trip{}, fmt.Errorf("read inserted trip: %w", classifyTripWriteError(err))
	}

	createdTrip, err := tripRow.ToTrip()
	if err != nil {
		return Trip{}, fmt.Errorf("decode inserted trip: %w", err)
	}
	return createdTrip, nil
}

func (r *repositoryImpl) GetById(ctx context.Context, id types.UUID) (Trip, error) {
	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	executor := postgres.Executor(ctx, r.pool)

	query, args, err := squirrel.Select(tripColumns).
		From("trips").
		Where(squirrel.Eq{"id": id}).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return Trip{}, fmt.Errorf("build get trip query: %w", err)
	}

	rows, err := executor.Query(ctx, query, args...)
	if err != nil {
		return Trip{}, fmt.Errorf("select trip: %w", err)
	}

	tripRow, err := pgx.CollectExactlyOneRow(
		rows, pgx.RowToStructByName[TripRow],
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, fmt.Errorf("select trip: %w: %w", TripNotFound, err)
	}
	if err != nil {
		return Trip{}, fmt.Errorf("read selected trip: %w", err)
	}

	trip, err := tripRow.ToTrip()
	if err != nil {
		return Trip{}, fmt.Errorf("decode selected trip: %w", err)
	}
	return trip, nil
}

func (r *repositoryImpl) UpdateState(ctx context.Context, trip Trip, from TripStatusState) (Trip, error) {
	if trip.status.State() == nil {
		return Trip{}, errors.New("update trip: missing status")
	}
	executor := postgres.Executor(ctx, r.pool)

	status := trip.status.State().String()

	query, args, err := squirrel.Update("trips").
		SetMap(map[string]any{
			"status":      status,
			"finished_at": trip.finishedAt,
			"updated_at":  squirrel.Expr("now()"),
		}).
		Where(squirrel.Eq{"id": trip.id, "status": from.String()}).
		PlaceholderFormat(squirrel.Dollar).
		Suffix("RETURNING " + tripColumns).
		ToSql()

	if err != nil {
		return Trip{}, fmt.Errorf("build update trip query: %w", err)
	}
	rows, err := executor.Query(ctx, query, args...)

	if err != nil {
		return Trip{}, fmt.Errorf("update trip: %w", classifyTripWriteError(err))
	}

	tripRow, err := pgx.CollectExactlyOneRow(
		rows, pgx.RowToStructByName[TripRow],
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Trip{}, fmt.Errorf("update trip: %w: %w", TripNotUpdated, err)
	}
	if err != nil {
		return Trip{}, fmt.Errorf("read updated trip: %w", classifyTripWriteError(err))
	}

	updatedTrip, err := tripRow.ToTrip()
	if err != nil {
		return Trip{}, fmt.Errorf("decode updated trip: %w", err)
	}
	return updatedTrip, nil
}

func (r *repositoryImpl) AddStatusHistory(
	ctx context.Context,
	id types.UUID,
	from *string,
	to string,
) error {
	query, args, err := squirrel.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status").
		Values(id, from, to).
		PlaceholderFormat(squirrel.Dollar).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip history query: %w", err)
	}

	_, err = postgres.Executor(ctx, r.pool).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("insert trip %s history: %w", id, err)
	}
	return nil
}

func classifyTripWriteError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "trips_one_active_driver_idx", "trips_driver_id_key":
		return fmt.Errorf("%w: %w", DriverBusy, err)
	default:
		return err
	}
}

func NewRepository(pool *pgxpool.Pool, queryTimeout time.Duration) Repository {
	return &repositoryImpl{pool: pool, queryTimeout: queryTimeout}
}
