package positions_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	"github.com/Penlk/avito-labs/internal/worker/positions"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type scannerFixture struct {
	scanner  *positions.Scanner
	queue    *positions.Queue
	selector *MockpushingCandidateSelector
	ticks    chan time.Time
	reader   *sdkmetric.ManualReader
	spans    *tracetest.InMemoryExporter
	logs     *bytes.Buffer
}

func newScannerFixture(t *testing.T, size int) *scannerFixture {
	t.Helper()
	controller := gomock.NewController(t)
	f := &scannerFixture{
		selector: NewMockpushingCandidateSelector(controller),
		ticks:    make(chan time.Time, 2),
		reader:   sdkmetric.NewManualReader(),
		spans:    tracetest.NewInMemoryExporter(),
		logs:     &bytes.Buffer{},
	}
	var err error
	f.queue, err = positions.NewQueue(size)
	if err != nil {
		t.Fatal(err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(f.reader))
	traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(f.spans))
	t.Cleanup(func() {
		ctx := context.WithoutCancel(t.Context())
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := traces.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	ticker := NewMockticker(controller)
	ticker.EXPECT().Ticks().Return(f.ticks)
	ticker.EXPECT().Stop()
	f.scanner, err = positions.NewScanner(f.selector, f.queue, positions.Options{
		CheckInterval: 30 * time.Second,
		StaleAfter:    time.Minute,
		Ticker:        ticker,
	}, slog.New(slog.NewJSONHandler(f.logs, nil)), provider.Meter("test"), traces.Tracer("test"))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func candidate(t *testing.T) trip.Trip {
	t.Helper()
	entity, err := trip.NewTrip(uuid.New(), uuid.New(), vo.Coordinates{}, vo.Price{}, vo.Coordinates{})
	if err != nil {
		t.Fatal(err)
	}
	return *entity
}

func counter(t *testing.T, reader *sdkmetric.ManualReader, name, result string) int64 {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			for _, point := range metric.Data.(metricdata.Sum[int64]).DataPoints {
				label, _ := point.Attributes.Value(attribute.Key("result"))
				if label.AsString() == result {
					return point.Value
				}
			}
		}
	}
	return 0
}

func wait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("scanner did not finish")
	}
}

func TestScanEnqueueResults(t *testing.T) {
	f := newScannerFixture(t, 2)
	first, second, third := candidate(t), candidate(t), candidate(t)
	gomock.InOrder(
		f.selector.EXPECT().Select(gomock.Any(), time.Minute).Return([]trip.Trip{first, second}, nil),
		f.selector.EXPECT().Select(gomock.Any(), time.Minute).Return([]trip.Trip{first, third}, nil),
	)
	f.ticks <- time.Time{}
	f.ticks <- time.Time{}
	close(f.ticks)
	f.scanner.Scan(t.Context())

	for result, want := range map[string]int64{"queued": 2, "duplicate": 1, "dropped": 1} {
		if got := counter(t, f.reader, "tripgo.stale_checks", result); got != want {
			t.Fatalf("checks result=%s count=%d want=%d", result, got, want)
		}
	}
	if got := counter(t, f.reader, "tripgo.position_requests", "dropped"); got != 1 {
		t.Fatalf("dropped requests=%d", got)
	}
	if !strings.Contains(f.logs.String(), `"level":"WARN"`) || !strings.Contains(f.logs.String(), "queue is full") {
		t.Fatalf("missing overflow warning: %s", f.logs.String())
	}
	spans := f.spans.GetSpans()
	if len(spans) != 2 {
		t.Fatalf("scan spans=%d", len(spans))
	}
	var count int
	for task := range f.queue.Tasks() {
		if !task.SpanContext().IsValid() || task.SpanContext().SpanID() != spans[0].SpanContext.SpanID() {
			t.Fatal("task lost its originating scan span")
		}
		entity := task.Trip()
		if entity.Id() != first.Id() && entity.Id() != second.Id() {
			t.Fatalf("unexpected queued trip=%s", entity.Id())
		}
		count++
	}
	if count != 2 {
		t.Fatalf("queued tasks=%d", count)
	}
}

func TestScanRecoversAfterSelectionError(t *testing.T) {
	f := newScannerFixture(t, 1)
	entity := candidate(t)
	gomock.InOrder(
		f.selector.EXPECT().Select(gomock.Any(), time.Minute).Return(nil, errors.New("database unavailable")),
		f.selector.EXPECT().Select(gomock.Any(), time.Minute).Return([]trip.Trip{entity}, nil),
	)
	f.ticks <- time.Time{}
	f.ticks <- time.Time{}
	close(f.ticks)
	f.scanner.Scan(t.Context())
	if f.queue.Len() != 1 || counter(t, f.reader, "tripgo.stale_checks", "queued") != 1 {
		t.Fatal("scanner did not recover on the next tick")
	}
	if strings.Count(f.logs.String(), `"error":"database unavailable"`) != 1 {
		t.Fatalf("selection error must be logged once: %s", f.logs.String())
	}
	spans := f.spans.GetSpans()
	if len(spans) != 2 || spans[0].Status.Code != codes.Error {
		t.Fatal("failed selection was not recorded in its span")
	}
	attributes := attribute.NewSet(spans[0].Attributes...)
	if value, ok := attributes.Value("error.type"); !ok || value.AsString() != "select_error" {
		t.Fatal("failed selection span has no error type")
	}
	attributes = attribute.NewSet(spans[1].Attributes...)
	if _, ok := attributes.Value("error.type"); ok || spans[1].Status.Code == codes.Error {
		t.Fatal("successful scan retained the previous scan error")
	}
}

func TestScanCancellationStopsInFlightSelection(t *testing.T) {
	f := newScannerFixture(t, 1)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	started := make(chan struct{})
	f.selector.EXPECT().Select(gomock.Any(), time.Minute).DoAndReturn(
		func(ctx context.Context, _ time.Duration) ([]trip.Trip, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.scanner.Scan(ctx)
	}()
	f.ticks <- time.Time{}
	wait(t, started)
	cancel()
	wait(t, done)
	if _, open := <-f.queue.Tasks(); open {
		t.Fatal("queue was not closed on shutdown")
	}
	if strings.Contains(f.logs.String(), `"level":"ERROR"`) {
		t.Fatalf("normal shutdown was logged as a failure: %s", f.logs.String())
	}
}

func TestScanTicksDoNotOverlap(t *testing.T) {
	f := newScannerFixture(t, 1)
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	release := make(chan struct{})
	var active atomic.Int32
	firstCall := f.selector.EXPECT().Select(gomock.Any(), time.Minute).DoAndReturn(
		func(ctx context.Context, _ time.Duration) ([]trip.Trip, error) {
			active.Add(1)
			defer active.Add(-1)
			close(firstStarted)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return nil, nil
		},
	)
	f.selector.EXPECT().Select(gomock.Any(), time.Minute).After(firstCall).DoAndReturn(
		func(context.Context, time.Duration) ([]trip.Trip, error) {
			if active.Add(1) != 1 {
				t.Error("selections overlap")
			}
			defer active.Add(-1)
			close(secondStarted)
			return nil, nil
		},
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.scanner.Scan(ctx)
	}()
	f.ticks <- time.Time{}
	wait(t, firstStarted)
	f.ticks <- time.Time{}
	close(release)
	wait(t, secondStarted)
	cancel()
	wait(t, done)
}
