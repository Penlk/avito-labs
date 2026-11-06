package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Penlk/avito-labs/internal/config"
	"github.com/Penlk/avito-labs/internal/container"
	"github.com/Penlk/avito-labs/internal/infra/telemetry"
)

func main() {
	if err := run(); err != nil {
		telemetry.LogError(context.Background(), os.Stdout, os.Getenv("OTEL_SERVICE_NAME"), err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.NewConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	app, err := container.New(ctx, cfg)
	if err != nil {
		return err
	}
	app.Logger.InfoContext(ctx, "service started", "address", cfg.HttpAddr)
	served := make(chan error, 1)
	go func() { served <- app.Server.ListenAndServe() }()
	select {
	case err = <-served:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	err = errors.Join(err, app.Close(shutdownCtx))
	if err == nil {
		app.Logger.InfoContext(shutdownCtx, "service stopped")
	}
	return err
}
