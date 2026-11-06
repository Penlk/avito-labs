package get

import (
	"net/http"

	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
	"github.com/Penlk/avito-labs/internal/http/response"
	triphttp "github.com/Penlk/avito-labs/internal/http/trip"
)

type Handler struct {
	usecase tripGetter
}

func New(usecase tripGetter) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	result, err := h.usecase.Get(r.Context(), tripId)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	response.WriteJSON(w, r, http.StatusOK, triphttp.ToDTO(result), nil)
}
