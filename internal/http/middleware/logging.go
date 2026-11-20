package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			bufferWriter := NewBufferWriter()

			oldTime := time.Now()
			defer func() {
				duration := time.Since(oldTime)

				rec := recover()
				if rec != nil {
					bufferWriter.status = http.StatusInternalServerError
				}

				route := chi.RouteContext(r.Context()).RoutePattern()
				if route == "" {
					route = unmatchedRoute
				}

				if bufferWriter.status >= 500 {
					requestLogger := logger
					if bufferWriter.err != nil {
						requestLogger = logger.With("error", bufferWriter.err)
					}
					requestLogger.ErrorContext(
						r.Context(),
						"request can't be served",
						"method", r.Method,
						"route", route,
						"status", bufferWriter.status,
						"duration_ms", duration.Milliseconds(),
					)
				} else {
					logger.InfoContext(
						r.Context(),
						"request handled",
						"method", r.Method,
						"route", route,
						"status", bufferWriter.status,
						"duration_ms", duration.Milliseconds(),
					)
				}

				if rec != nil {
					panic(rec)
				}

				if err := bufferWriter.WriteToOriginal(w); err != nil {
					logger.ErrorContext(r.Context(), "write response failed", "error", err)
				}
			}()
			handler.ServeHTTP(bufferWriter, r)
		})
	}
}
