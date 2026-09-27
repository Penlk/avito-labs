package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Penlk/avito-labs/internal/postgres"
	"github.com/Penlk/avito-labs/internal/trips"
	tripshttp "github.com/Penlk/avito-labs/internal/trips/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	queryTimeout, err := time.ParseDuration(os.Getenv("DATABASE_QUERY_TIMEOUT"))
	if err != nil || queryTimeout <= 0 {
		panic("DATABASE_QUERY_TIMEOUT must be a positive duration")
	}

	plConfig, err := pgxpool.ParseConfig("postgres://postgres:postgres@localhost:5432/postgres")
	if err != nil {
		panic(err)
	}

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

	repository := trips.NewRepository(pool)
	txManager := postgres.NewTxManager(pool, queryTimeout)
	service := trips.NewService(repository, txManager)
	handler := tripshttp.NewHandler(service, pool, queryTimeout)

	server := &http.Server{
		Addr:    ":8080", // Значение HTTP_ADDR, например ":8080".
		Handler: handler.Router(),
	}
	go server.ListenAndServe()
	<-appCtx.Done()
}
