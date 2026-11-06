package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/go-chi/chi/v5"

	"github.com/Penlk/avito-labs/internal/http/domainerror"
	"github.com/Penlk/avito-labs/internal/http/requeststate"
)

func Recover(logger *slog.Logger, routes chi.Routes) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			state := &requeststate.State{Route: route(routes, r), Status: http.StatusOK}
			r = r.WithContext(requeststate.With(r.Context(), state))
			state.TraceContext = r.Context()
			buffer := &responseBuffer{header: make(http.Header), state: state}
			defer func() {
				if value := recover(); value != nil {
					logger.ErrorContext(state.TraceContext, "request panic",
						"error", value, "stack", string(debug.Stack()))
					buffer = &responseBuffer{header: make(http.Header), state: state}
					domainerror.Write(buffer, r, domainerror.InternalError)
				}
				for key, values := range buffer.header {
					w.Header()[key] = values
				}
				w.WriteHeader(state.Status)
				if _, err := w.Write(buffer.body.Bytes()); err != nil {
					logger.WarnContext(state.TraceContext, "write response failed", "error", err)
				}
			}()
			next.ServeHTTP(buffer, r)
		})
	}
}

func route(routes chi.Routes, r *http.Request) string {
	ctx := chi.NewRouteContext()
	routes.Match(ctx, r.Method, r.URL.Path)
	if pattern := ctx.RoutePattern(); pattern != "" {
		return pattern
	}
	return "/unmatched"
}

type responseBuffer struct {
	header      http.Header
	body        bytes.Buffer
	state       *requeststate.State
	wroteHeader bool
}

func (w *responseBuffer) Header() http.Header { return w.header }
func (w *responseBuffer) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.state.Status = status
}

func (w *responseBuffer) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}
