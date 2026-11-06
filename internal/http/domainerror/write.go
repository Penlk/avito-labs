package domainerror

import (
	"encoding/json"
	"net/http"

	"github.com/Penlk/avito-labs/internal/http/requeststate"
)

func Write(w http.ResponseWriter, r *http.Request, err error) {
	status, problem := Problem(err)
	problem.Instance = new(r.URL.Path)
	requeststate.Record(r.Context(), err, problem.Code)

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(problem); err != nil {
		requeststate.Record(r.Context(), err, "response_write")
	}
}
