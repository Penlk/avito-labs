//go:build integration

package trip_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	domain "github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
	repository "github.com/Penlk/avito-labs/internal/repository/trip"
	"github.com/Penlk/avito-labs/internal/usecase/trip/finish"
)

const activeStatus = "active"

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

func newTrip(t *testing.T, driver uuid.UUID) domain.Trip {
	t.Helper()
	point, err := vo.NewCoordinates(55, 37)
	if err != nil {
		t.Fatal(err)
	}
	price, err := vo.NewPrice(1000)
	if err != nil {
		t.Fatal(err)
	}
	entity, err := domain.NewTrip(driver, uuid.New(), point, price, point)
	if err != nil {
		t.Fatal(err)
	}
	return *entity
}

func TestRepositoryLifecycle(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, 3*time.Second)
	tx := transaction.NewTxManager(pool, 3*time.Second)
	ctx := t.Context()
	entity := newTrip(t, uuid.New())
	err := tx.Do(ctx, func(ctx context.Context) error {
		created, err := repo.Create(ctx, entity)
		if err != nil {
			return err
		}
		if created.Id() != entity.Id() || created.Price() != entity.Price() {
			t.Fatal("insert changed trip")
		}
		return repo.AddStatusHistory(ctx, entity.Id(), nil, activeStatus)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetById(ctx, entity.Id())
	if err != nil || got.State().String() != activeStatus {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	_, err = repo.Create(ctx, newTrip(t, entity.DriverId()))
	if !errors.Is(err, domain.ErrDriverBusy) {
		t.Fatalf("busy=%v", err)
	}
	_, err = repo.GetById(ctx, uuid.New())
	if !errors.Is(err, domain.ErrTripNotFound) {
		t.Fatalf("missing=%v", err)
	}
	at := time.Now().UTC().Truncate(time.Microsecond)
	if err := entity.Finish(at); err != nil {
		t.Fatal(err)
	}
	err = tx.Do(ctx, func(ctx context.Context) error {
		updated, err := repo.UpdateState(ctx, entity, domain.ActiveState{})
		if err != nil {
			return err
		}
		if !updated.FinishedAt().Equal(at) {
			t.Fatal("finish time lost")
		}
		from := activeStatus
		return repo.AddStatusHistory(ctx, entity.Id(), &from, "completed")
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.UpdateState(ctx, entity, domain.ActiveState{})
	if !errors.Is(err, finish.ErrTripNotUpdated) {
		t.Fatalf("repeat update=%v", err)
	}
	assertCounts(t, pool, entity.Id(), 1, 2)
	if _, err := repo.Create(ctx, newTrip(t, entity.DriverId())); err != nil {
		t.Fatalf("driver not released: %v", err)
	}
}

func TestTransactionRollsBackBothInserts(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, 3*time.Second)
	tx := transaction.NewTxManager(pool, 3*time.Second)
	for _, invalidHistory := range []bool{false, true} {
		entity := newTrip(t, uuid.New())
		failed := errors.New("fail after both inserts")
		err := tx.Do(t.Context(), func(ctx context.Context) error {
			if _, err := repo.Create(ctx, entity); err != nil {
				return err
			}
			state := activeStatus
			if invalidHistory {
				state = "invalid"
			}
			if err := tx.Do(ctx, func(ctx context.Context) error {
				return repo.AddStatusHistory(ctx, entity.Id(), nil, state)
			}); err != nil {
				return err
			}
			return failed
		})
		if err == nil || (!invalidHistory && !errors.Is(err, failed)) {
			t.Fatalf("rollback error=%v", err)
		}
		assertCounts(t, pool, entity.Id(), 0, 0)
	}
}

func TestTransactionRollsBackFinish(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, 3*time.Second)
	tx := transaction.NewTxManager(pool, 3*time.Second)
	entity := newTrip(t, uuid.New())
	if _, err := repo.Create(t.Context(), entity); err != nil {
		t.Fatal(err)
	}
	if err := entity.Finish(time.Now()); err != nil {
		t.Fatal(err)
	}
	err := tx.Do(t.Context(), func(ctx context.Context) error {
		if _, err := repo.UpdateState(ctx, entity, domain.ActiveState{}); err != nil {
			return err
		}
		return repo.AddStatusHistory(ctx, entity.Id(), nil, "invalid")
	})
	if err == nil {
		t.Fatal("expected history constraint failure")
	}
	got, err := repo.GetById(t.Context(), entity.Id())
	if err != nil || got.State().String() != activeStatus || got.FinishedAt() != nil {
		t.Fatalf("finish did not roll back: %+v %v", got, err)
	}
	assertCounts(t, pool, entity.Id(), 1, 0)
}

func assertCounts(t *testing.T, pool *pgxpool.Pool, id uuid.UUID, trips, history int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var gotTrips, gotHistory int
	err := pool.QueryRow(ctx, `SELECT
  (SELECT count(*) FROM trips WHERE id=$1),
  (SELECT count(*) FROM trip_status_history WHERE trip_id=$1)`, id).Scan(&gotTrips, &gotHistory)
	if err != nil {
		t.Fatal(err)
	}
	if gotTrips != trips || gotHistory != history {
		t.Fatalf("rows trips=%d history=%d, want %d/%d", gotTrips, gotHistory, trips, history)
	}
}

func TestCountActive(t *testing.T) {
	pool := database(t)
	repo := repository.NewRepository(pool, 3*time.Second)
	ctx := t.Context()
	assertActive := func(want int64) {
		t.Helper()
		got, err := repo.CountActive(ctx)
		if err != nil || got != want {
			t.Fatalf("active=%d err=%v want=%d", got, err, want)
		}
	}
	assertActive(0)
	first, second := newTrip(t, uuid.New()), newTrip(t, uuid.New())
	for _, trip := range []domain.Trip{first, second} {
		if _, err := repo.Create(ctx, trip); err != nil {
			t.Fatal(err)
		}
	}
	assertActive(2)
	if err := first.Finish(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateState(ctx, first, domain.ActiveState{}); err != nil {
		t.Fatal(err)
	}
	assertActive(1)
	// A new repository instance must see existing rows, not process-local counters.
	repo = repository.NewRepository(pool, 3*time.Second)
	assertActive(1)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.CountActive(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
