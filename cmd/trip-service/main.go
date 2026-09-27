package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
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
	
	rt := chi.NewRouter()
	rt.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	rt.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		if err := pool.Ping(appCtx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	})

	server := &http.Server{
		Addr:    ":8080", // Значение HTTP_ADDR, например ":8080".
		Handler: rt,
	}
	go server.ListenAndServe()
	<-appCtx.Done()
}