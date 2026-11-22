package get

import (
	"net/http"

	"github.com/google/uuid"

	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
	"github.com/Penlk/avito-labs/internal/http/position"
	"github.com/Penlk/avito-labs/internal/http/response"
)

type Handler struct {
	positionLister PositionLister
}

func New(positionLister PositionLister) *Handler {
	return &Handler{
		positionLister: positionLister,
	}
}

func (h *Handler) ListTripPositions(w http.ResponseWriter, r *http.Request, tripId uuid.UUID) {
	result, err := h.positionLister.GetByTripId(r.Context(), tripId)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	resultJson := make([]api.TripPosition, len(result))
	for i, pos := range result {
		resultJson[i] = position.ToDTO(pos)
	}

	response.WriteJSON(w, r, http.StatusOK, api.TripPositionsResponse{Positions: resultJson}, nil)
}
