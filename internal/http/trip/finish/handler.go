package finish

import (
	"io"
	"net/http"

	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
	"github.com/Penlk/avito-labs/internal/http/response"
	triphttp "github.com/Penlk/avito-labs/internal/http/trip"
)

type Handler struct {
	usecase tripFinisher
}

func New(usecase tripFinisher) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	if n, err := r.Body.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		domainerror.Write(w, r, domainerror.InvalidRequest.WithDetail("body must be empty"))
		return
	}
	result, err := h.usecase.Finish(r.Context(), tripId)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	response.WriteJSON(w, r, http.StatusOK, triphttp.ToDTO(result), nil)
}
