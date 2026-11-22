//go:build integration

package position_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/Penlk/avito-labs/internal/domain/position"
	"github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
	repository "github.com/Penlk/avito-labs/internal/repository/position"
	triprepo "github.com/Penlk/avito-labs/internal/repository/trip"
	"github.com/Penlk/avito-labs/internal/usecase/position/create"
)

const queryTimeout = 3 * time.Second

func database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required: run make test-integration")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{"test_" + strings.ReplaceAll(uuid.NewString(), "-", "")}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(t.Context()), 10*time.Second)
		defer cleanupCancel()
		defer admin.Close()
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	files, err := filepath.Glob("../../../migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("migration files: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		_, up, ok := strings.Cut(string(data), "-- +goose Up")
		if !ok {
			t.Fatalf("missing Up in %s", file)
		}
		up, _, _ = strings.Cut(up, "-- +goose Down")
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("migrate %s: %v", file, err)
		}
	}
	return pool
}

func newTrip(t *testing.T, repo *triprepo.Repository) trip.Trip {
	t.Helper()
	coordinates, err := vo.NewCoordinates(55, 37)
	if err != nil {
		t.Fatal(err)
	}
	price, err := vo.NewPrice(1000)
	if err != nil {
		t.Fatal(err)
	}
	entity, err := trip.NewTrip(uuid.New(), uuid.New(), coordinates, price, coordinates)
	if err != nil {
		t.Fatal(err)
	}
	created, err := repo.Create(t.Context(), *entity)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func newPosition(t *testing.T, tripID uuid.UUID, at time.Time) domain.Position {
	t.Helper()
	coordinates, err := vo.NewCoordinates(55, 37)
	if err != nil {
		t.Fatal(err)
	}
	pos, err := domain.NewPosition(tripID, coordinates, at)
	if err != nil {
		t.Fatal(err)
	}
	return *pos
}

func TestRepositoryRoute(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, queryTimeout)
	trips := triprepo.NewRepository(pool, queryTimeout)
	entity := newTrip(t, trips)
	other := newTrip(t, trips)
	ctx := t.Context()

	positions, err := repo.GetByTripId(ctx, entity.Id())
	if err != nil || positions == nil || len(positions) != 0 {
		t.Fatalf("empty route=%+v err=%v", positions, err)
	}
	_, err = repo.GetByTripId(ctx, uuid.New())
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("missing trip=%v", err)
	}

	base := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	times := []time.Time{base.Add(time.Second), base, base.Add(time.Second)}
	created := make([]domain.Position, len(times))
	for i, at := range times {
		pos, err := repo.Add(ctx, newPosition(t, entity.Id(), at))
		if err != nil {
			t.Fatal(err)
		}
		if pos.Id() <= 0 || pos.TripId() != entity.Id() || !pos.RecordedAt().Equal(at) ||
			pos.Coordinates().Latitude() != 55 || pos.Coordinates().Longitude() != 37 {
			t.Fatalf("inserted position=%+v", pos)
		}
		created[i] = pos
	}
	if _, err := repo.Add(ctx, newPosition(t, other.Id(), base)); err != nil {
		t.Fatal(err)
	}
	positions, err = repo.GetByTripId(ctx, entity.Id())
	if err != nil || len(positions) != len(times) {
		t.Fatalf("route=%+v err=%v", positions, err)
	}
	for i, source := range []int{1, 0, 2} {
		if positions[i].Id() != created[source].Id() {
			t.Fatalf("route order at %d: id=%d want=%d", i, positions[i].Id(), created[source].Id())
		}
	}
	_, err = repo.Add(ctx, newPosition(t, uuid.New(), base))
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("insert for missing trip=%v", err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.GetByTripId(canceled, entity.Id()); !errors.Is(err, context.Canceled) {
		t.Fatalf("read cancellation lost: %v", err)
	}
	if _, err := repo.Add(canceled, newPosition(t, entity.Id(), base)); !errors.Is(err, context.Canceled) {
		t.Fatalf("write cancellation lost: %v", err)
	}
}

func TestCreateKeepsNewestPosition(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, queryTimeout)
	trips := triprepo.NewRepository(pool, queryTimeout)
	tx := transaction.NewTxManager(pool, queryTimeout)
	usecase := create.New(tx, repo, trips)
	entity := newTrip(t, trips)
	newest := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	for _, at := range []time.Time{newest, newest.Add(-time.Minute)} {
		if _, err := usecase.Create(t.Context(), newPosition(t, entity.Id(), at)); err != nil {
			t.Fatal(err)
		}
		got, err := trips.GetById(t.Context(), entity.Id())
		if err != nil || got.LastPositionAt() == nil || !got.LastPositionAt().Equal(newest) {
			t.Fatalf("last position=%v err=%v", got.LastPositionAt(), err)
		}
	}
	_, err := usecase.Create(t.Context(), newPosition(t, uuid.New(), newest))
	if !errors.Is(err, trip.ErrTripNotFound) {
		t.Fatalf("missing trip=%v", err)
	}
	if err := entity.Finish(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := trips.UpdateState(t.Context(), entity, trip.ActiveState{}); err != nil {
		t.Fatal(err)
	}
	_, err = usecase.Create(t.Context(), newPosition(t, entity.Id(), newest))
	if !errors.Is(err, trip.ErrTripCompleted) {
		t.Fatalf("completed trip=%v", err)
	}
	positions, err := repo.GetByTripId(t.Context(), entity.Id())
	if err != nil || len(positions) != 2 {
		t.Fatalf("completed route=%+v err=%v", positions, err)
	}
}

func TestCreateRollsBackOnInsertFailure(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, queryTimeout)
	trips := triprepo.NewRepository(pool, queryTimeout)
	tx := transaction.NewTxManager(pool, queryTimeout)
	usecase := create.New(tx, repo, trips)
	entity := newTrip(t, trips)
	previous := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	if _, err := usecase.Create(t.Context(), newPosition(t, entity.Id(), previous)); err != nil {
		t.Fatal(err)
	}
	_, err := pool.Exec(t.Context(), "ALTER TABLE trip_positions ADD CONSTRAINT reject_positions CHECK (false) NOT VALID")
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.Create(t.Context(), newPosition(t, entity.Id(), previous.Add(time.Second)))
	if err == nil {
		t.Fatal("expected insert failure")
	}
	got, err := trips.GetById(t.Context(), entity.Id())
	if err != nil || got.LastPositionAt() == nil || !got.LastPositionAt().Equal(previous) {
		t.Fatalf("timestamp did not roll back: %v err=%v", got.LastPositionAt(), err)
	}
	positions, err := repo.GetByTripId(t.Context(), entity.Id())
	if err != nil || len(positions) != 1 {
		t.Fatalf("insert did not roll back: %+v err=%v", positions, err)
	}
}

func TestOuterTransactionRollsBackPosition(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, queryTimeout)
	trips := triprepo.NewRepository(pool, queryTimeout)
	tx := transaction.NewTxManager(pool, queryTimeout)
	usecase := create.New(tx, repo, trips)
	entity := newTrip(t, trips)
	pos := newPosition(t, entity.Id(), time.Now().UTC().Add(-time.Minute))
	failed := errors.New("fail after saving position")
	err := tx.Do(t.Context(), func(ctx context.Context) error {
		if _, err := usecase.Create(ctx, pos); err != nil {
			return err
		}
		return failed
	})
	if !errors.Is(err, failed) {
		t.Fatalf("transaction error=%v", err)
	}
	got, err := trips.GetById(t.Context(), entity.Id())
	if err != nil || got.LastPositionAt() != nil {
		t.Fatalf("timestamp did not roll back: %v err=%v", got.LastPositionAt(), err)
	}
	positions, err := repo.GetByTripId(t.Context(), entity.Id())
	if err != nil || len(positions) != 0 {
		t.Fatalf("position did not roll back: %+v err=%v", positions, err)
	}
}

func TestConcurrentPositionsKeepNewestTimestamp(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, queryTimeout)
	trips := triprepo.NewRepository(pool, queryTimeout)
	tx := transaction.NewTxManager(pool, queryTimeout)
	usecase := create.New(tx, repo, trips)
	entity := newTrip(t, trips)
	newest := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	positions := []domain.Position{
		newPosition(t, entity.Id(), newest),
		newPosition(t, entity.Id(), newest.Add(-time.Minute)),
	}
	start := make(chan struct{})
	errorsCh := make(chan error, len(positions))
	var workers sync.WaitGroup
	for _, pos := range positions {
		workers.Go(func() {
			<-start
			_, err := usecase.Create(t.Context(), pos)
			errorsCh <- err
		})
	}
	close(start)
	workers.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := trips.GetById(t.Context(), entity.Id())
	if err != nil || got.LastPositionAt() == nil || !got.LastPositionAt().Equal(newest) {
		t.Fatalf("concurrent timestamp=%v err=%v", got.LastPositionAt(), err)
	}
	route, err := repo.GetByTripId(t.Context(), entity.Id())
	if err != nil || len(route) != len(positions) {
		t.Fatalf("concurrent route=%+v err=%v", route, err)
	}
}

func TestPositionMigrationDownAndUp(t *testing.T) {
	pool := database(t)
	data, err := os.ReadFile("../../../migrations/00004_add_trip_positions.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, up, ok := strings.Cut(string(data), "-- +goose Up")
	if !ok {
		t.Fatal("missing Up")
	}
	up, down, ok := strings.Cut(up, "-- +goose Down")
	if !ok {
		t.Fatal("missing Down")
	}
	if _, err := pool.Exec(t.Context(), down); err != nil {
		t.Fatal(err)
	}
	var table *string
	if err := pool.QueryRow(t.Context(), "SELECT to_regclass('trip_positions')::text").Scan(&table); err != nil {
		t.Fatal(err)
	}
	if table != nil {
		t.Fatalf("positions table still exists: %s", *table)
	}
	if _, err := pool.Exec(t.Context(), up); err != nil {
		t.Fatal(err)
	}
	trips := triprepo.NewRepository(pool, queryTimeout)
	entity := newTrip(t, trips)
	repo := repository.NewRepository(pool, queryTimeout)
	if _, err := repo.Add(t.Context(), newPosition(t, entity.Id(), time.Now().UTC().Add(-time.Minute))); err != nil {
		t.Fatal(err)
	}
}
