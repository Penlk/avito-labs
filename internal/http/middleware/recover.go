package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/Penlk/avito-labs/internal/http/domainerror"
)

func Recover(logger *slog.Logger) func(handler http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buffer := NewBufferWriter()

			defer func() {
				if rec := recover(); rec != nil {
					domainerror.Write(w, r, domainerror.InternalError)
					logger.ErrorContext(r.Context(), "request panic", "panic", rec, "stack", string(debug.Stack()))
					return
				}

				if err := buffer.WriteToOriginal(w); err != nil {
					logger.ErrorContext(r.Context(), "write response failed", "error", err)
				}
			}()

			handler.ServeHTTP(buffer, r)
		})
	}
}
