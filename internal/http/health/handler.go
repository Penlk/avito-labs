package health

import (
	"net/http"

	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/response"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	response.WriteJSON(w, r, http.StatusOK, api.HealthResponse{Status: api.Ok}, nil)
}
