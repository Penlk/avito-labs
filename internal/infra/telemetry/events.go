package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"
)

type Events struct {
	logger    *slog.Logger
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func NewEvents(meter metric.Meter, logger *slog.Logger) (*Events, error) {
	created, err := meter.Int64Counter("tripgo.trips.created")
	if err != nil {
		return nil, fmt.Errorf("created counter: %w", err)
	}
	completed, err := meter.Int64Counter("tripgo.trips.completed")
	if err != nil {
		return nil, fmt.Errorf("completed counter: %w", err)
	}
	return &Events{logger: logger, created: created, completed: completed}, nil
}

func (e *Events) Created(ctx context.Context, id uuid.UUID) {
	e.created.Add(ctx, 1)
	e.logger.InfoContext(ctx, "trip created", "trip_id", id.String())
}

func (e *Events) Completed(ctx context.Context, id uuid.UUID) {
	e.completed.Add(ctx, 1)
	e.logger.InfoContext(ctx, "trip completed", "trip_id", id.String())
}
