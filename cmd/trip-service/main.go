package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Penlk/avito-labs/internal/postgres"
	"github.com/Penlk/avito-labs/internal/trips"
	tripshttp "github.com/Penlk/avito-labs/internal/trips/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	config, err := NewConfig()
	if err != nil {
		panic(err)
	}

	plConfig, err := pgxpool.ParseConfig(config.DatabaseUrl)
	if err != nil {
		panic(err)
	}

	plConfig.MaxConns = int32(config.DatabaseMaxConns)
	plConfig.MinConns = int32(config.DatabaseMinConns)
	plConfig.MaxConnLifetime = config.DatabaseMaxConnLifetime
	plConfig.ConnConfig.ConnectTimeout = config.DatabaseConnectTimeout

	appCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	pool, err := pgxpool.NewWithConfig(appCtx, plConfig)
	if err != nil {
		panic(err)
	}

	pingCtx, cancelPing := context.WithTimeout(appCtx, config.DatabaseConnectTimeout)
	err = pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		pool.Close()
		panic(fmt.Errorf("ping database: %w", err))
	}

	repository := trips.NewRepository(pool, config.DatabaseQueryTimeout)
	txManager := postgres.NewTxManager(pool, config.DatabaseQueryTimeout)
	service := trips.NewService(repository, txManager)
	handler := tripshttp.NewHandler(service, pool, config.DatabaseQueryTimeout)

	server := &http.Server{
		Addr:              config.HttpAddr,
		Handler:           handler.Router(),
		ReadTimeout:       config.HTTPReadTimeout,
		ReadHeaderTimeout: config.HTTPReadHeaderTimeout,
		WriteTimeout:      config.HTTPWriteTimeout,
		IdleTimeout:       config.HTTPIdleTimeout,
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.ListenAndServe()
	}()

	select {
	case err := <-errChan:
		panic(err)
	case <-appCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			config.ShutdownTimeout,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			fmt.Fprintln(os.Stderr, "HTTP shutdown failed:", err)
			os.Exit(1)
		}

		poolClosed := make(chan struct{})
		go func() {
			pool.Close()
			close(poolClosed)
		}()

		select {
		case <-poolClosed:
			return
		case <-shutdownCtx.Done():
			fmt.Fprintln(os.Stderr, "Shutdown timeout:", shutdownCtx.Err())
			os.Exit(1)
		}
	}
}
