package positions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/Penlk/avito-labs/internal/gateway/push"
)

type Worker struct {
	pusher   pusher
	timeout  time.Duration
	logger   *slog.Logger
	requests metric.Int64Counter
	duration metric.Float64Histogram
	tracer   trace.Tracer
	active   atomic.Int64
}

func NewWorker(
	pusher pusher, timeout time.Duration, logger *slog.Logger, meter metric.Meter, tracer trace.Tracer,
) (*Worker, error) {
	if timeout <= 0 {
		return nil, errors.New("push request timeout must be positive")
	}
	if pusher == nil || logger == nil || meter == nil || tracer == nil {
		return nil, errors.New("worker dependencies must not be nil")
	}
	requests, err := meter.Int64Counter("tripgo.position_requests")
	if err != nil {
		return nil, fmt.Errorf("position requests counter: %w", err)
	}
	duration, err := meter.Float64Histogram("tripgo.position_requests.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 1.05, 1.1, 1.25, 1.5, 2.5, 5, 10))
	if err != nil {
		return nil, fmt.Errorf("position requests duration: %w", err)
	}
	worker := &Worker{pusher: pusher, timeout: timeout, logger: logger, requests: requests, duration: duration, tracer: tracer}
	_, err = meter.Int64ObservableGauge("tripgo.position_requests.workers.active",
		metric.WithInt64Callback(func(_ context.Context, observer metric.Int64Observer) error {
			observer.Observe(worker.active.Load())
			return nil
		}))
	if err != nil {
		return nil, fmt.Errorf("active position workers gauge: %w", err)
	}
	return worker, nil
}

func (w *Worker) Work(ctx context.Context, que *Queue) {
	w.logger.InfoContext(ctx, "position worker started")
	defer w.logger.InfoContext(ctx, "position worker stopped")
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case task, open := <-que.Tasks():
			if !open || ctx.Err() != nil {
				return
			}
			if !que.Start(task) {
				continue
			}
			w.process(ctx, que, task)
		}
	}
}

func (w *Worker) process(ctx context.Context, que *Queue, task *Task) {
	defer que.Complete(task)
	w.active.Add(1)
	defer w.active.Add(-1)

	taskCtx, cancel := context.WithTimeout(ctx, w.timeout)
	defer cancel()
	options := []trace.SpanStartOption{trace.WithNewRoot()}
	if origin := task.SpanContext(); origin.IsValid() {
		options = append(options, trace.WithLinks(trace.Link{SpanContext: origin}))
	}
	taskCtx, span := w.tracer.Start(taskCtx, "positions.process", options...)
	defer span.End()
	trip := task.Trip()
	req := push.NewRequest(uuid.NewString(), trip.DriverId().String(), push.Data{TripId: trip.Id()})
	pushCtx, pushSpan := w.tracer.Start(taskCtx, "push.request", trace.WithSpanKind(trace.SpanKindClient))
	defer pushSpan.End()
	started := time.Now()
	_, err := w.pusher.Push(pushCtx, req)
	elapsed := time.Since(started).Seconds()
	result := "success"
	if err != nil {
		result = "error"
		for _, current := range []trace.Span{span, pushSpan} {
			current.RecordError(err)
			current.SetStatus(codes.Error, "request position")
			current.SetAttributes(attribute.String("error.type", "push_error"))
		}
		if ctx.Err() == nil {
			w.logger.WarnContext(taskCtx, "push service request failed", "trip_id", trip.Id(), "error", err)
		}
	}
	labels := metric.WithAttributes(attribute.String("result", result))
	w.requests.Add(taskCtx, 1, labels)
	w.duration.Record(taskCtx, elapsed, labels)
}
