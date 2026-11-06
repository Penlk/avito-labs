package container

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/Penlk/avito-labs/internal/config"
	transport "github.com/Penlk/avito-labs/internal/http"
	"github.com/Penlk/avito-labs/internal/http/health"
	"github.com/Penlk/avito-labs/internal/http/middleware"
	"github.com/Penlk/avito-labs/internal/http/ready"
	createhttp "github.com/Penlk/avito-labs/internal/http/trip/create"
	finishhttp "github.com/Penlk/avito-labs/internal/http/trip/finish"
	gethttp "github.com/Penlk/avito-labs/internal/http/trip/get"
	"github.com/Penlk/avito-labs/internal/infra/postgres"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
	"github.com/Penlk/avito-labs/internal/infra/telemetry"
	triprepo "github.com/Penlk/avito-labs/internal/repository/trip"
	createusecase "github.com/Penlk/avito-labs/internal/usecase/trip/create"
	finishusecase "github.com/Penlk/avito-labs/internal/usecase/trip/finish"
	getusecase "github.com/Penlk/avito-labs/internal/usecase/trip/get"
)

type Container struct {
	Server    *http.Server
	Logger    *slog.Logger
	pool      *pgxpool.Pool
	telemetry *telemetry.Providers
}

func New(ctx context.Context, cfg *config.Config) (*Container, error) {
	logger, err := telemetry.NewLogger(os.Stdout, cfg.LogLevel, cfg.ServiceName)
	if err != nil {
		return nil, fmt.Errorf("logger: %w", err)
	}
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.WarnContext(ctx, "telemetry export failed", "error", err)
	}))
	providers, err := telemetry.New(ctx, cfg.OTLPProtocol, cfg.ServiceName)
	if err != nil {
		return nil, err
	}
	app := &Container{Logger: logger, telemetry: providers}
	if err := app.build(ctx, cfg); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.ShutdownTimeout)
		defer cancel()
		return nil, errors.Join(err, app.Close(cleanupCtx))
	}
	return app, nil
}

func (c *Container) build(ctx context.Context, cfg *config.Config) error {
	plConfig, err := pgxpool.ParseConfig(cfg.DatabaseUrl)
	if err != nil {
		return errors.New("invalid DATABASE_URL")
	}
	plConfig.MaxConns = cfg.DatabaseMaxConns
	plConfig.MinConns = cfg.DatabaseMinConns
	plConfig.MaxConnLifetime = cfg.DatabaseMaxConnLifetime
	plConfig.ConnConfig.ConnectTimeout = cfg.DatabaseConnectTimeout
	plConfig.ConnConfig.Tracer, err = postgres.NewTracer(c.telemetry.Traces, c.telemetry.Metrics)
	if err != nil {
		return err
	}
	c.pool, err = pgxpool.NewWithConfig(ctx, plConfig)
	if err != nil {
		return fmt.Errorf("create database pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, cfg.DatabaseConnectTimeout)
	defer cancel()
	if err := c.pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	events, err := telemetry.NewEvents(c.telemetry.Metrics.Meter("tripgo/trips"), c.Logger)
	if err != nil {
		return err
	}
	metrics, err := middleware.NewMetrics(c.telemetry.Metrics.Meter("tripgo/http"))
	if err != nil {
		return err
	}
	repository := triprepo.NewRepository(c.pool, cfg.DatabaseQueryTimeout)
	if err := telemetry.RegisterActiveTrips(
		c.telemetry.Metrics.Meter("tripgo/trips"), repository,
	); err != nil {
		return err
	}
	txManager := transaction.NewTxManager(c.pool, cfg.DatabaseQueryTimeout)
	handler := transport.NewRouter(
		createhttp.New(createusecase.New(repository, repository, txManager, events)),
		gethttp.New(getusecase.New(repository)),
		finishhttp.New(finishusecase.New(repository, repository, repository, txManager, events)),
		health.New(), ready.New(c.pool, cfg.DatabaseQueryTimeout))
	routes := handler.(chi.Routes)
	handler = middleware.Recover(c.Logger, routes)(
		middleware.Tracing(c.telemetry.Traces)(
			middleware.Logging(c.Logger)(metrics.Handler(handler))))
	c.Server = &http.Server{
		Addr: cfg.HttpAddr, Handler: handler,
		ReadTimeout:       cfg.HTTPReadTimeout,
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout, IdleTimeout: cfg.HTTPIdleTimeout,
	}
	return nil
}

func (c *Container) Close(ctx context.Context) error {
	var err error
	if c.Server != nil {
		err = c.Server.Shutdown(ctx)
		if err != nil {
			err = errors.Join(err, c.Server.Close())
		}
	}

	// Metric callbacks need the database during the final collection.
	if exportErr := c.telemetry.Shutdown(ctx); exportErr != nil {
		c.Logger.WarnContext(ctx, "telemetry shutdown failed", "error", exportErr)
	}
	if c.pool != nil {
		closed := make(chan struct{})
		go func() { c.pool.Close(); close(closed) }()
		select {
		case <-closed:
		case <-ctx.Done():
			err = errors.Join(err, ctx.Err())
		}
	}
	return errors.Join(err, ctx.Err())
}
