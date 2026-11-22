package create

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/Penlk/avito-labs/internal/http/domainerror"
	positionhttp "github.com/Penlk/avito-labs/internal/http/position"
	"github.com/Penlk/avito-labs/internal/http/request"
	"github.com/Penlk/avito-labs/internal/http/response"
)

type Handler struct {
	positionCreator PositionCreator
}

func New(positionCreator PositionCreator) *Handler {
	return &Handler{
		positionCreator: positionCreator,
	}
}

func (h *Handler) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripId uuid.UUID) {
	var body requestBody
	if err := request.DecodeJSON(r.Body, &body); err != nil {
		domainerror.Write(w, r, domainerror.InvalidRequest.WithDetail(jsonErrorDetail(err)))
		return
	}

	pos, err := toDomain(body, tripId)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	result, err := h.positionCreator.Create(r.Context(), *pos)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	response.WriteJSON(w, r, http.StatusCreated, positionhttp.ToDTO(result), nil)
}
