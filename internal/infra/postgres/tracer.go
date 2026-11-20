package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"

	"github.com/Penlk/avito-labs/internal/infra/telemetry"
)

type Tracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}

type (
	queryKey struct{}
	query    struct {
		start                 time.Time
		operation, collection string
	}
)

func NewTracer(tp trace.TracerProvider, mp metric.MeterProvider) (*Tracer, error) {
	duration, err := mp.Meter("tripgo/postgres").Float64Histogram(
		"db.client.operation.duration", metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(telemetry.DurationBuckets...))
	if err != nil {
		return nil, fmt.Errorf("database histogram: %w", err)
	}
	return &Tracer{
		Tracer: otelpgx.NewTracer(otelpgx.WithTracerProvider(tp),
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
			otelpgx.WithTrimSQLInSpanName(),
			otelpgx.WithDisableConnectionDetailsInAttributes(),
			otelpgx.WithDisableSQLStatementInAttributes()),
		duration: duration,
	}, nil
}

func (t *Tracer) TraceQueryStart(
	ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData,
) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)
	operation, collection := queryAttributes(data.SQL)
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("db.operation.name", operation),
		attribute.String("db.collection.name", collection))
	return context.WithValue(ctx, queryKey{}, query{time.Now(), operation, collection})
}

func (t *Tracer) TraceQueryEnd(
	ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData,
) {
	q, ok := ctx.Value(queryKey{}).(query)
	if !ok {
		t.Tracer.TraceQueryEnd(ctx, conn, data)
		return
	}
	errorType := ""
	if data.Err != nil {
		errorType = fmt.Sprintf("%T", data.Err)
		var pgErr *pgconn.PgError
		if errors.As(data.Err, &pgErr) {
			errorType = pgErr.Code
		}
		span := trace.SpanFromContext(ctx)
		span.SetStatus(codes.Error, "database operation failed")
		span.SetAttributes(attribute.String("error.type", errorType))
	}
	t.duration.Record(ctx, time.Since(q.start).Seconds(), metric.WithAttributes(
		attribute.String("db.operation.name", q.operation),
		attribute.String("db.collection.name", q.collection),
		attribute.String("error.type", errorType)))
	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

// Only bounded SQL keywords/table names become labels; never query values.
func queryAttributes(sql string) (string, string) {
	words := strings.Fields(strings.ToLower(sql))
	if len(words) == 0 {
		return "OTHER", ""
	}
	operation := strings.ToUpper(words[0])
	switch operation {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "BEGIN", "COMMIT", "ROLLBACK":
	default:
		operation = "OTHER"
	}
	for _, word := range words {
		table := strings.Trim(word, "\"(),")
		if table == "trips" || table == "trip_status_history" {
			return operation, table
		}
	}
	return operation, ""
}
