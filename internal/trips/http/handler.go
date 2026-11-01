package tripshttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/Penlk/avito-labs/api"
	"github.com/Penlk/avito-labs/internal/trips"
	"github.com/Penlk/avito-labs/internal/trips/vo"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	service      trips.Service
	db           Pinger
	readyTimeout time.Duration
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var body api.CreateTripJSONRequestBody
	if err := decoder.Decode(&body); err != nil {
		writeProblem(w, r, InvalidRequest.WithDetail(jsonErrorDetail(err)))
		return
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		writeProblem(w, r, InvalidRequest.WithDetail("expected one JSON object"))
		return
	}

	if body.StartPoint.Latitude == nil || body.StartPoint.Longitude == nil {
		writeProblem(w, r, InvalidRequest.WithDetail("start_point coordinates are required"))
		return
	}
	if body.EndPoint.Latitude == nil || body.EndPoint.Longitude == nil {
		writeProblem(w, r, InvalidRequest.WithDetail("end_point coordinates are required"))
		return
	}
	if body.Price == nil {
		writeProblem(w, r, InvalidRequest.WithDetail("price is required"))
		return
	}

	startPoint, err := vo.NewCoordinates(*body.StartPoint.Latitude, *body.StartPoint.Longitude)
	if err != nil {
		writeProblem(w, r, InvalidRequest.WithDetail("start_point: "+err.Error()))
		return
	}
	endPoint, err := vo.NewCoordinates(*body.EndPoint.Latitude, *body.EndPoint.Longitude)
	if err != nil {
		writeProblem(w, r, InvalidRequest.WithDetail("end_point: "+err.Error()))
		return
	}
	price, err := vo.NewPrice(*body.Price)
	if err != nil {
		writeProblem(w, r, InvalidRequest.WithDetail(err.Error()))
		return
	}

	trip, err := trips.NewTrip(body.DriverId, body.UserId, endPoint, price, startPoint)
	if err != nil {
		writeProblem(w, r, InvalidRequest.WithDetail(err.Error()))
		return
	}

	result, err := h.service.Create(r.Context(), *trip)
	if err != nil {
		writeProblem(w, r, err)
		return
	}

	writeResponse(w, r, http.StatusCreated, TripToDTo(result), map[string]string{
		"Location": "/api/v1/trips/" + result.Id().String(),
	})
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	if n, err := r.Body.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		writeProblem(w, r, InvalidRequest.WithDetail("body must be empty"))
		return
	}
	result, err := h.service.Finish(r.Context(), tripId)
	if err != nil {
		writeProblem(w, r, err)
		return
	}

	writeResponse(w, r, http.StatusOK, TripToDTo(result), nil)
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	result, err := h.service.Get(r.Context(), tripId)
	if err != nil {
		writeProblem(w, r, err)
		return
	}

	writeResponse(w, r, http.StatusOK, TripToDTo(result), nil)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeResponse(w, r, http.StatusOK, api.HealthResponse{Status: api.Ok}, nil)
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.readyTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		body := api.HealthResponse{Status: api.Unavailable}
		writeResponse(w, r, http.StatusServiceUnavailable, body, nil)
		return
	}

	writeResponse(w, r, http.StatusOK, api.HealthResponse{Status: api.Ok}, nil)
}

var _ api.ServerInterface = &Handler{}

func NewHandler(service trips.Service, db Pinger, timeout time.Duration) *Handler {
	return &Handler{service: service, db: db, readyTimeout: timeout}
}

func (h *Handler) Router() http.Handler {
	return api.HandlerWithOptions(h, api.ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, r, InvalidRequest.WithDetail(err.Error()))
		},
	})
}

func writeResponse(
	w http.ResponseWriter, r *http.Request, status int,
	jsonObject any, header map[string]string,
) {
	data, err := json.Marshal(jsonObject)
	if err != nil {
		writeProblem(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	for k, v := range header {
		w.Header().Set(k, v)
	}
	w.WriteHeader(status)

	if _, err := w.Write(data); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeProblem(w http.ResponseWriter, r *http.Request, err error) {
	problem := errToRfc(err).WithInstance(r.URL.Path).Problem
	if problem.Status >= http.StatusInternalServerError {
		log.Printf("handle %s %s: %v", r.Method, r.URL.Path, err)
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(int(problem.Status))

	if err := json.NewEncoder(w).Encode(problem); err != nil {
		log.Printf("write problem response: %v", err)
	}
}

func errToRfc(err error) RfcError {
	var problem RfcError
	if errors.As(err, &problem) {
		return problem
	}

	switch {
	case errors.Is(err, trips.TripNotFound):
		return TripNotFound
	case errors.Is(err, trips.TripCompleted):
		return TripCompleted
	case errors.Is(err, trips.DriverBusy):
		return DriverBusy
	default:
		return InternalError
	}
}
