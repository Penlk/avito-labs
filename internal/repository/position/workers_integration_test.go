//go:build integration

package position_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/Penlk/avito-labs/internal/gateway/push"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
	positionrepo "github.com/Penlk/avito-labs/internal/repository/position"
	triprepo "github.com/Penlk/avito-labs/internal/repository/trip"
	"github.com/Penlk/avito-labs/internal/scanner"
	"github.com/Penlk/avito-labs/internal/usecase/position/create"
	"github.com/Penlk/avito-labs/internal/workers"
)

type singleTick struct{ ticks <-chan time.Time }

func (t singleTick) Ticks() <-chan time.Time { return t.ticks }
func (singleTick) Stop()                     {}

func TestScanSavePositionAndDispatch(t *testing.T) {
	pool := database(t)
	trips := triprepo.NewRepository(pool, queryTimeout)
	updated, stale := newTrip(t, trips), newTrip(t, trips)
	if _, err := pool.Exec(t.Context(), "UPDATE trips SET started_at = NOW() - INTERVAL '2 minutes'"); err != nil {
		t.Fatal(err)
	}
	queue, err := scanner.NewQueue(10)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	meter := noop.NewMeterProvider().Meter("test")
	tracer := tracenoop.NewTracerProvider().Tracer("test")
	ticks := make(chan time.Time, 1)
	ticks <- time.Now()
	close(ticks)
	scan, err := scanner.NewScanner(trips, queue, scanner.Options{
		CheckInterval: time.Second, StaleAfter: time.Minute,
		NewTicker: func(time.Duration) scanner.Ticker { return singleTick{ticks} },
	}, logger, meter, tracer)
	if err != nil {
		t.Fatal(err)
	}
	scan.Scan(t.Context())
	if queue.Len() != 2 {
		t.Fatalf("queued=%d; want two stale trips", queue.Len())
	}
	positions := positionrepo.NewRepository(pool, queryTimeout)
	usecase := create.New(transaction.NewTxManager(pool, queryTimeout), positions, trips, queue, noopEvents{})
	if _, err := usecase.Create(t.Context(), newPosition(t, updated.Id(), time.Now())); err != nil {
		t.Fatal(err)
	}
	requests := make(chan push.Request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request push.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- request
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"message_id":"accepted","accepted_at":"2026-11-22T16:42:22Z"}`)
	}))
	t.Cleanup(server.Close)
	gateway := push.NewGateway(server.URL, time.Second)
	t.Cleanup(gateway.Close)
	worker, err := workers.NewWorker(gateway, time.Second, logger, meter, tracer)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	worker.Work(ctx, queue)
	if len(requests) != 1 {
		t.Fatalf("push requests=%d; want only the trip without a new position", len(requests))
	}
	if request := <-requests; request.Data.TripId != stale.Id() || request.RecipientId != stale.DriverId().String() {
		t.Fatalf("unexpected request: %+v", request)
	}
}
