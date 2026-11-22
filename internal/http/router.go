package http

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	api "github.com/Penlk/avito-labs/internal/generated"
	"github.com/Penlk/avito-labs/internal/http/domainerror"
	"github.com/Penlk/avito-labs/internal/http/health"
	httpposcreate "github.com/Penlk/avito-labs/internal/http/position/create"
	httpposget "github.com/Penlk/avito-labs/internal/http/position/get"
	"github.com/Penlk/avito-labs/internal/http/ready"
	"github.com/Penlk/avito-labs/internal/http/trip/create"
	"github.com/Penlk/avito-labs/internal/http/trip/finish"
	"github.com/Penlk/avito-labs/internal/http/trip/get"
)

type router struct {
	createTrip *create.Handler
	getTrip    *get.Handler
	finishTrip *finish.Handler

	createPosition *httpposcreate.Handler
	listPositions  *httpposget.Handler

	health *health.Handler
	ready  *ready.Handler
}

func NewRouter(
	createHandler *create.Handler,
	getHandler *get.Handler,
	finishHandler *finish.Handler,
	createPosition *httpposcreate.Handler,
	listPositions *httpposget.Handler,
	healthHandler *health.Handler,
	readyHandler *ready.Handler,
	middlewares []func(http.Handler) http.Handler,
) http.Handler {
	handlers := &router{
		createTrip: createHandler,
		getTrip:    getHandler,
		finishTrip: finishHandler,

		createPosition: createPosition,
		listPositions:  listPositions,

		health: healthHandler,
		ready:  readyHandler,
	}
	r := chi.NewRouter()
	r.Use(middlewares...)
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		domainerror.Write(w, req, domainerror.NotFound)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		methods := []string{
			http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete,
			http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch,
		}
		path := req.URL.RawPath
		if path == "" {
			path = req.URL.Path
		}
		allowed := make([]string, 0, len(methods))
		for _, method := range methods {
			if r.Match(chi.NewRouteContext(), method, path) {
				allowed = append(allowed, method)
			}
		}
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		domainerror.Write(w, req, domainerror.MethodNotAllowed)
	})
	return api.HandlerWithOptions(handlers, api.ChiServerOptions{
		BaseRouter: r,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			domainerror.Write(w, r, domainerror.InvalidRequest.WithDetail(err.Error()))
		},
	})
}

func (h *router) CreateTrip(
	w http.ResponseWriter, r *http.Request, params api.CreateTripParams,
) {
	h.createTrip.CreateTrip(w, r, params)
}

func (h *router) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	h.getTrip.GetTrip(w, r, tripID)
}

func (h *router) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	h.finishTrip.FinishTrip(w, r, tripID)
}

func (h *router) Health(w http.ResponseWriter, r *http.Request) {
	h.health.Health(w, r)
}

func (h *router) Ready(w http.ResponseWriter, r *http.Request) {
	h.ready.Ready(w, r)
}

func (h *router) CreateTripPosition(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	h.createPosition.CreateTripPosition(w, r, tripId)
}

func (h *router) ListTripPositions(w http.ResponseWriter, r *http.Request, tripId api.TripId) {
	h.listPositions.ListTripPositions(w, r, tripId)
}

var _ api.ServerInterface = (*router)(nil)
