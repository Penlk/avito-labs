package workers_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/Penlk/avito-labs/internal/domain/trip"
	"github.com/Penlk/avito-labs/internal/domain/trip/vo"
	"github.com/Penlk/avito-labs/internal/gateway/push"
	"github.com/Penlk/avito-labs/internal/scanner"
	"github.com/Penlk/avito-labs/internal/workers"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

type fixture struct {
	worker *workers.Worker
	pusher *MockPusher
	queue  *scanner.Queue
	reader *sdkmetric.ManualReader
	spans  *tracetest.InMemoryExporter
	tracer trace.Tracer
	logs   *bytes.Buffer
}

func newFixture(t *testing.T, timeout time.Duration) *fixture {
	t.Helper()
	f := &fixture{
		pusher: NewMockPusher(gomock.NewController(t)), reader: sdkmetric.NewManualReader(),
		spans: tracetest.NewInMemoryExporter(), logs: &bytes.Buffer{},
	}
	var err error
	f.queue, err = scanner.NewQueue(10)
	if err != nil {
		t.Fatal(err)
	}
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(f.reader))
	traces := sdktrace.NewTracerProvider(sdktrace.WithSyncer(f.spans))
	f.tracer = traces.Tracer("test")
	t.Cleanup(func() {
		f.queue.Close()
		ctx := context.WithoutCancel(t.Context())
		if err := provider.Shutdown(ctx); err != nil {
			t.Error(err)
		}
		if err := traces.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	f.worker, err = workers.NewWorker(f.pusher, timeout, slog.New(slog.NewJSONHandler(f.logs, nil)),
		provider.Meter("test"), f.tracer)
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

func wait(t *testing.T, done <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), 5*time.Second)
	defer cancel()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("worker did not finish")
	}
}

func start(t *testing.T, f *fixture, count int) (context.CancelFunc, <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	var pool sync.WaitGroup
	for range count {
		pool.Go(func() { f.worker.Work(ctx, f.queue) })
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		pool.Wait()
	}()
	t.Cleanup(func() {
		cancel()
		wait(t, done)
	})
	return cancel, done
}

func measurement(t *testing.T, f *fixture, name, result string) float64 {
	t.Helper()
	var data metricdata.ResourceMetrics
	if err := f.reader.Collect(t.Context(), &data); err != nil {
		t.Fatal(err)
	}
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != name {
				continue
			}
			switch values := metric.Data.(type) {
			case metricdata.Gauge[int64]:
				return float64(values.DataPoints[0].Value)
			case metricdata.Sum[int64]:
				for _, point := range values.DataPoints {
					label, _ := point.Attributes.Value(attribute.Key("result"))
					if label.AsString() == result {
						return float64(point.Value)
					}
				}
			case metricdata.Histogram[float64]:
				for _, point := range values.DataPoints {
					label, _ := point.Attributes.Value(attribute.Key("result"))
					if label.AsString() == result {
						return float64(point.Count)
					}
				}
			}
		}
	}
	return 0
}

func TestWorkContinuesAfterErrorAndCompletesTasks(t *testing.T) {
	f := newFixture(t, time.Second)
	first, second := candidate(t), candidate(t)
	scanCtx, origin := f.tracer.Start(t.Context(), "positions.scan")
	if f.queue.TryAdd(scanCtx, first) != scanner.Queued || f.queue.TryAdd(scanCtx, second) != scanner.Queued {
		t.Fatal("tasks were not queued")
	}
	origin.End()
	gomock.InOrder(
		f.pusher.EXPECT().Push(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req push.Request) (push.Response, error) {
				if req.Data.TripId != first.Id() || req.RecipientId != first.DriverId().String() || req.RequestId == "" {
					t.Errorf("unexpected request: %+v", req)
				}
				return push.Response{}, errors.New("push unavailable")
			},
		),
		f.pusher.EXPECT().Push(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, req push.Request) (push.Response, error) {
				if req.Data.TripId != second.Id() {
					t.Errorf("second task was not processed: %+v", req)
				}
				if f.queue.TryAdd(scanCtx, first) != scanner.Queued {
					t.Error("failed task remained in deduplication state")
				}
				f.queue.Close()
				return push.Response{}, nil
			},
		),
		f.pusher.EXPECT().Push(gomock.Any(), gomock.Any()).Return(push.Response{}, nil),
	)
	f.worker.Work(t.Context(), f.queue)
	if measurement(t, f, "tripgo.position_requests", "error") != 1 ||
		measurement(t, f, "tripgo.position_requests", "success") != 2 ||
		measurement(t, f, "tripgo.position_requests.duration", "error") != 1 ||
		measurement(t, f, "tripgo.position_requests.duration", "success") != 2 {
		t.Fatal("push results or durations were not recorded")
	}
	if measurement(t, f, "tripgo.position_requests.workers.active", "") != 0 {
		t.Fatal("worker remained active after draining")
	}
	if strings.Count(f.logs.String(), `"error":"push unavailable"`) != 1 {
		t.Fatalf("push error must be logged once: %s", f.logs.String())
	}
	var jobs, calls, failedJobs, failedCalls int
	for _, span := range f.spans.GetSpans() {
		switch span.Name {
		case "positions.process":
			jobs++
			if span.Parent.IsValid() || len(span.Links) != 1 ||
				span.Links[0].SpanContext.SpanID() != origin.SpanContext().SpanID() ||
				span.SpanContext.TraceID() == origin.SpanContext().TraceID() {
				t.Error("worker span must link to the scan from a new trace")
			}
			if span.Status.Code == codes.Error {
				failedJobs++
			}
		case "push.request":
			calls++
			if span.SpanKind != trace.SpanKindClient || !span.Parent.IsValid() {
				t.Error("push call must be a child client span")
			}
			if span.Status.Code == codes.Error {
				failedCalls++
			}
		}
	}
	if jobs != 3 || calls != 3 || failedJobs != 1 || failedCalls != 1 {
		t.Fatalf("jobs=%d calls=%d failed jobs=%d failed calls=%d", jobs, calls, failedJobs, failedCalls)
	}
}

func TestWorkSkipsCanceledTask(t *testing.T) {
	f := newFixture(t, time.Second)
	canceled, active := candidate(t), candidate(t)
	f.queue.TryAdd(t.Context(), canceled)
	f.queue.Cancel(canceled.Id())
	f.queue.TryAdd(t.Context(), active)
	f.queue.Close()
	f.pusher.EXPECT().Push(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, req push.Request) (push.Response, error) {
			if req.Data.TripId != active.Id() {
				t.Errorf("canceled trip was sent: %+v", req)
			}
			return push.Response{}, nil
		},
	)
	f.worker.Work(t.Context(), f.queue)
	if measurement(t, f, "tripgo.position_requests", "success") != 1 {
		t.Fatal("canceled task was counted as a push attempt")
	}
}

func TestWorkHasSeparateTimeoutForEachTask(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const timeout = 25 * time.Millisecond
		f := newFixture(t, timeout)
		f.queue.TryAdd(t.Context(), candidate(t))
		f.queue.TryAdd(t.Context(), candidate(t))
		f.queue.Close()
		gomock.InOrder(
			f.pusher.EXPECT().Push(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, _ push.Request) (push.Response, error) {
					deadline, ok := ctx.Deadline()
					if !ok || deadline.After(time.Now().Add(timeout)) {
						t.Error("task has no per-request deadline")
					}
					<-ctx.Done()
					return push.Response{}, ctx.Err()
				},
			),
			f.pusher.EXPECT().Push(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, _ push.Request) (push.Response, error) {
					if ctx.Err() != nil {
						t.Error("previous timeout canceled the next task")
					}
					return push.Response{}, nil
				},
			),
		)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		f.worker.Work(ctx, f.queue)
		if measurement(t, f, "tripgo.position_requests", "error") != 1 ||
			measurement(t, f, "tripgo.position_requests", "success") != 1 {
			t.Fatal("worker stopped after a task timeout")
		}
	})
}

func TestWorkCancellation(t *testing.T) {
	for _, busy := range []bool{false, true} {
		name := "idle"
		if busy {
			name = "busy"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, time.Second)
			entity := candidate(t)
			started := make(chan struct{})
			if busy {
				f.queue.TryAdd(t.Context(), entity)
				f.pusher.EXPECT().Push(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, _ push.Request) (push.Response, error) {
						close(started)
						<-ctx.Done()
						return push.Response{}, ctx.Err()
					},
				)
			}
			cancel, done := start(t, f, 1)
			if busy {
				wait(t, started)
				if measurement(t, f, "tripgo.position_requests.workers.active", "") != 1 {
					t.Fatal("busy worker was not observed")
				}
			}
			cancel()
			wait(t, done)
			if busy && f.queue.TryAdd(t.Context(), entity) != scanner.Queued {
				t.Fatal("canceled in-flight task remained in deduplication state")
			}
		})
	}
}

func TestPoolDrainsWithFixedConcurrency(t *testing.T) {
	const count = 3
	const tasks = 7
	f := newFixture(t, 5*time.Second)
	for range tasks {
		f.queue.TryAdd(t.Context(), candidate(t))
	}
	f.queue.Close()
	started := make(chan struct{}, tasks)
	release := make(chan struct{})
	var active, peak atomic.Int64
	f.pusher.EXPECT().Push(gomock.Any(), gomock.Any()).Times(tasks).DoAndReturn(
		func(ctx context.Context, _ push.Request) (push.Response, error) {
			current := active.Add(1)
			defer active.Add(-1)
			for previous := peak.Load(); current > previous; previous = peak.Load() {
				if peak.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return push.Response{}, ctx.Err()
			}
			return push.Response{}, nil
		},
	)
	_, done := start(t, f, count)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for range count {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("pool did not start all workers")
		}
	}
	if measurement(t, f, "tripgo.position_requests.workers.active", "") != count || f.queue.Len() != tasks-count {
		t.Fatal("pool has an unexpected number of active and queued tasks")
	}
	close(release)
	wait(t, done)
	if peak.Load() != count || measurement(t, f, "tripgo.position_requests", "success") != tasks {
		t.Fatal("pool did not process all tasks with fixed concurrency")
	}
}
