package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Penlk/avito-labs/internal/http/requeststate"
)

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			state := requeststate.From(r.Context())
			completed := false
			defer func() {
				if !completed {
					state.Status = http.StatusInternalServerError
				}
				level := slog.LevelInfo
				attrs := []any{
					"method", r.Method, "route", state.Route,
					"status", state.Status, "duration_ms", float64(time.Since(started)) / float64(time.Millisecond),
				}
				if state.Status >= http.StatusInternalServerError {
					level = slog.LevelError
					if state.Err != nil {
						attrs = append(attrs, "error", state.Err)
					}
				}
				logger.Log(r.Context(), level, "http request", attrs...)
			}()
			next.ServeHTTP(w, r)
			completed = true
		})
	}
}
