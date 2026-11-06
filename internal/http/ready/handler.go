package ready

import (
	"context"
	"net/http"
	"time"

	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/requeststate"
	"github.com/Penlk/avito-labs/internal/http/response"
)

type Handler struct {
	db           pinger
	readyTimeout time.Duration
}

func New(db pinger, timeout time.Duration) *Handler {
	return &Handler{db: db, readyTimeout: timeout}
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.readyTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		requeststate.Record(r.Context(), err, "database_unavailable")
		body := api.HealthResponse{Status: api.Unavailable}
		response.WriteJSON(w, r, http.StatusServiceUnavailable, body, nil)
		return
	}

	response.WriteJSON(w, r, http.StatusOK, api.HealthResponse{Status: api.Ok}, nil)
}
