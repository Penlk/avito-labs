package create

import (
	"encoding/json"
	"io"
	"net/http"

	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
	"github.com/Penlk/avito-labs/internal/http/response"
	triphttp "github.com/Penlk/avito-labs/internal/http/trip"
)

type Handler struct {
	usecase tripCreator
}

func New(usecase tripCreator) *Handler {
	return &Handler{usecase: usecase}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var body api.CreateTripJSONRequestBody
	if err := decoder.Decode(&body); err != nil {
		domainerror.Write(w, r, domainerror.InvalidRequest.WithDetail(jsonErrorDetail(err)))
		return
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		domainerror.Write(w, r, domainerror.InvalidRequest.WithDetail("expected one JSON object"))
		return
	}

	trip, err := toDomain(body)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	result, err := h.usecase.Create(r.Context(), *trip)
	if err != nil {
		domainerror.Write(w, r, err)
		return
	}

	response.WriteJSON(w, r, http.StatusCreated, triphttp.ToDTO(result), map[string]string{
		"Location": "/api/v1/trips/" + result.Id().String(),
	})
}
