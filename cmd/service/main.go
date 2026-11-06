package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Penlk/avito-labs/internal/config"
	transport "github.com/Penlk/avito-labs/internal/http"
	"github.com/Penlk/avito-labs/internal/http/health"
	"github.com/Penlk/avito-labs/internal/http/ready"
	createhttp "github.com/Penlk/avito-labs/internal/http/trip/create"
	finishhttp "github.com/Penlk/avito-labs/internal/http/trip/finish"
	gethttp "github.com/Penlk/avito-labs/internal/http/trip/get"
	transaction "github.com/Penlk/avito-labs/internal/infra/postgres/transaction"
	triprepo "github.com/Penlk/avito-labs/internal/repository/trip"
	createusecase "github.com/Penlk/avito-labs/internal/usecase/trip/create"
	finishusecase "github.com/Penlk/avito-labs/internal/usecase/trip/finish"
	getusecase "github.com/Penlk/avito-labs/internal/usecase/trip/get"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	config, err := config.NewConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	plConfig, err := pgxpool.ParseConfig(config.DatabaseUrl)
	if err != nil {
		return fmt.Errorf("parse database config: %w", err)
	}

	plConfig.MaxConns = config.DatabaseMaxConns
	plConfig.MinConns = config.DatabaseMinConns
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
		return fmt.Errorf("create database pool: %w", err)
	}

	pingCtx, cancelPing := context.WithTimeout(appCtx, config.DatabaseConnectTimeout)
	err = pool.Ping(pingCtx)
	cancelPing()
	if err != nil {
		pool.Close()
		return fmt.Errorf("ping database: %w", err)
	}

	repository := triprepo.NewRepository(pool, config.DatabaseQueryTimeout)
	txManager := transaction.NewTxManager(pool, config.DatabaseQueryTimeout)
	createTrip := createusecase.NewTripGetter(repository, repository, txManager)
	getTrip := getusecase.New(repository)
	finishTrip := finishusecase.New(repository, repository, repository, txManager)
	handler := transport.NewRouter(
		createhttp.New(createTrip),
		gethttp.New(getTrip),
		finishhttp.New(finishTrip),
		health.New(),
		ready.New(pool, config.DatabaseQueryTimeout),
	)

	server := &http.Server{
		Addr:              config.HttpAddr,
		Handler:           handler,
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
		return fmt.Errorf("serve HTTP: %w", err)
	case <-appCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			config.ShutdownTimeout,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("HTTP shutdown failed: %w", err)
		}

		poolClosed := make(chan struct{})
		go func() {
			pool.Close()
			close(poolClosed)
		}()

		select {
		case <-poolClosed:
			return nil
		case <-shutdownCtx.Done():
			return fmt.Errorf("shutdown timeout: %w", shutdownCtx.Err())
		}
	}
}
