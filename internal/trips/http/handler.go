package tripshttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Penlk/avito-labs/api"
	"github.com/Penlk/avito-labs/internal/trips"
	"github.com/Penlk/avito-labs/internal/trips/vo"
	"github.com/google/uuid"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	service      trips.Service
	db           Pinger
	readyTimeout time.Duration
}

var _ api.ServerInterface = &Handler{}

func NewHandler(service trips.Service, db Pinger, timeout time.Duration) *Handler {
	return &Handler{service: service, db: db, readyTimeout: timeout}
}

func (h *Handler) Router() http.Handler {
	return api.HandlerWithOptions(h, api.ChiServerOptions{
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		},
	})
}

func (h *Handler) CreateTrip(
	w http.ResponseWriter,
	r *http.Request,
	_ api.CreateTripParams,
) {
	body, err := decodeCreateRequest(r)
	if err != nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	input := trips.Trip{
		UserId:   body.UserId,
		DriverId: body.DriverId,
		Price:    body.Price,
		StartPoint: vo.Coordinates{
			Latitude: body.StartPoint.Latitude, Longitude: body.StartPoint.Longitude,
		},
		EndPoint: vo.Coordinates{
			Latitude: body.EndPoint.Latitude, Longitude: body.EndPoint.Longitude,
		},
	}

	trip, err := h.service.Create(r.Context(), input)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeTrip(w, r, http.StatusCreated, trip)
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	if id == uuid.Nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Empty trip ID")
		return
	}
	trip, err := h.service.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeTrip(w, r, http.StatusOK, trip)
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, id api.TripId) {
	if id == uuid.Nil {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Empty trip ID")
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil || len(bytes.TrimSpace(body)) != 0 {
		writeProblem(w, r, http.StatusBadRequest, "invalid_request", "Body must be empty")
		return
	}

	trip, err := h.service.UpdateState(r.Context(), id, trips.CompletedState{})
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	writeTrip(w, r, http.StatusOK, trip)
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, "application/json", api.HealthResponse{
		Status: api.Ok,
	})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.readyTimeout)
	defer cancel()

	status, bodyStatus := http.StatusOK, api.Ok
	if err := h.db.Ping(ctx); err != nil {
		status, bodyStatus = http.StatusServiceUnavailable, api.Unavailable
	}
	writeJSON(w, r, status, "application/json", api.HealthResponse{
		Status: bodyStatus,
	})
}

func decodeCreateRequest(r *http.Request) (api.CreateTripJSONRequestBody, error) {
	var body api.CreateTripJSONRequestBody
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return body, errors.New("cannot read request body")
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return body, err
	}

	fields, err := requiredFields(data,
		"user_id", "driver_id", "start_point", "end_point", "price",
	)
	if err != nil {
		return body, err
	}
	for _, name := range []string{"start_point", "end_point"} {
		if _, err := requiredFields(fields[name], "latitude", "longitude"); err != nil {
			return body, fmt.Errorf("%s: %w", name, err)
		}
	}
	if body.UserId == uuid.Nil || body.DriverId == uuid.Nil {
		return body, errors.New("user_id and driver_id must be nonzero UUIDs")
	}
	if body.Price < 0 {
		return body, errors.New("price must be nonnegative")
	}
	for _, point := range []api.Coordinates{body.StartPoint, body.EndPoint} {
		if point.Latitude < -90 || point.Latitude > 90 ||
			point.Longitude < -180 || point.Longitude > 180 {
			return body, errors.New("coordinates are out of range")
		}
	}
	return body, nil
}

func requiredFields(data []byte, names ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for name := range fields {
		if !slices.Contains(names, name) {
			return nil, fmt.Errorf("unknown field %q", name)
		}
	}
	for _, name := range names {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("%s is required and cannot be null", name)
		}
	}
	return fields, nil
}

func toAPITrip(trip trips.Trip) (api.Trip, error) {
	state := trip.Status.GetState()
	if state == nil {
		return api.Trip{}, errors.New("trip has no status")
	}
	status := api.TripStatus(state.String())
	if !status.Valid() {
		return api.Trip{}, errors.New("trip has an unknown status")
	}
	return api.Trip{
		Id: trip.Id, UserId: trip.UserId, DriverId: trip.DriverId,
		Price: trip.Price, Status: status,
		StartedAt: trip.StartedAt, FinishedAt: trip.FinishedAt,
		LastPositionAt: trip.LastPositionAt,
		StartPoint: api.Coordinates{
			Latitude: trip.StartPoint.Latitude, Longitude: trip.StartPoint.Longitude,
		},
		EndPoint: api.Coordinates{
			Latitude: trip.EndPoint.Latitude, Longitude: trip.EndPoint.Longitude,
		},
	}, nil
}

func writeTrip(w http.ResponseWriter, r *http.Request, status int, trip trips.Trip) {
	body, err := toAPITrip(trip)
	if err != nil {
		writeServiceError(w, r, err)
		return
	}
	if status == http.StatusCreated {
		w.Header().Set("Location", "/api/v1/trips/"+trip.Id.String())
	}
	writeJSON(w, r, status, "application/json", body)
}

func writeServiceError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, trips.TripNotFound):
		writeProblem(w, r, http.StatusNotFound, "trip_not_found", "Trip not found")
	case errors.Is(err, trips.DriverBusy):
		writeProblem(w, r, http.StatusConflict, "driver_busy", "Driver has an active trip")
	case errors.Is(err, trips.TripCompleted):
		writeProblem(w, r, http.StatusConflict, "trip_completed", "Trip already completed")
	default:
		writeProblem(w, r, http.StatusInternalServerError,
			"internal_error", "Internal server error")
	}
}

func writeProblem(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	code, detail string,
) {
	instance := r.URL.Path
	writeJSON(w, r, status, "application/problem+json", api.Problem{
		Type:  "https://tripgo.example/problems/" + strings.ReplaceAll(code, "_", "-"),
		Title: http.StatusText(status), Status: int32(status), Code: code,
		Detail: &detail, Instance: &instance,
	})
}

func writeJSON(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	contentType string,
	body any,
) {
	data, err := json.Marshal(body)
	if err != nil {
		w.Header().Del("Location")
		writeServiceError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
