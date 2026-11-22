package scanner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type Options struct {
	CheckInterval time.Duration
	StaleAfter    time.Duration
	NewTicker     func(time.Duration) Ticker
}

type Scanner struct {
	selector      PushingCandidateSelector
	checkInterval time.Duration
	staleAfter    time.Duration
	queue         *Queue
	logger        *slog.Logger
	staleChecks   metric.Int64Counter
	requests      metric.Int64Counter
	tracer        trace.Tracer
	newTicker     func(time.Duration) Ticker
}

func NewScanner(
	selector PushingCandidateSelector, queue *Queue, options Options,
	logger *slog.Logger, meter metric.Meter, tracer trace.Tracer,
) (*Scanner, error) {
	if options.CheckInterval <= 0 || options.StaleAfter <= 0 {
		return nil, errors.New("position check interval and stale threshold must be positive")
	}
	if selector == nil || queue == nil || logger == nil || meter == nil || tracer == nil {
		return nil, errors.New("scanner dependencies must not be nil")
	}
	staleChecks, err := meter.Int64Counter("tripgo.stale_checks")
	if err != nil {
		return nil, fmt.Errorf("stale checks counter: %w", err)
	}
	requests, err := meter.Int64Counter("tripgo.position_requests")
	if err != nil {
		return nil, fmt.Errorf("position requests counter: %w", err)
	}
	newTicker := options.NewTicker
	if newTicker == nil {
		newTicker = func(interval time.Duration) Ticker { return systemTicker{time.NewTicker(interval)} }
	}
	return &Scanner{
		selector: selector, queue: queue, checkInterval: options.CheckInterval, staleAfter: options.StaleAfter,
		logger: logger, staleChecks: staleChecks, requests: requests, tracer: tracer, newTicker: newTicker,
	}, nil
}

func (s *Scanner) Scan(ctx context.Context) {
	defer s.queue.Close()
	if ctx.Err() != nil {
		return
	}
	ticker := s.newTicker(s.checkInterval)
	defer ticker.Stop()
	ticks := ticker.Ticks()
	s.logger.InfoContext(ctx, "position scanner started")
	defer s.logger.InfoContext(ctx, "position scanner stopped")
	for {
		select {
		case <-ctx.Done():
			return
		case _, open := <-ticks:
			if !open || ctx.Err() != nil {
				return
			}
			s.scan(ctx)
		}
	}
}

func (s *Scanner) scan(ctx context.Context) {
	ctx, span := s.tracer.Start(ctx, "positions.scan")
	defer span.End()
	trips, err := s.selector.Select(ctx, s.staleAfter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "select stale trips")
		if ctx.Err() == nil {
			s.logger.ErrorContext(ctx, "select stale trips", "error", err)
		}
		return
	}
	for _, trip := range trips {
		if ctx.Err() != nil {
			return
		}
		result := s.queue.TryAdd(ctx, trip)
		labels := metric.WithAttributes(attribute.String("result", string(result)))
		s.staleChecks.Add(ctx, 1, labels)
		if result == Dropped {
			s.requests.Add(ctx, 1, labels)
			s.logger.WarnContext(ctx, "position request dropped: queue is full", "trip_id", trip.Id())
		}
	}
}

type systemTicker struct{ *time.Ticker }

func (t systemTicker) Ticks() <-chan time.Time { return t.C }
