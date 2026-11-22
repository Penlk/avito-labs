package domainerror

import (
	"encoding/json"
	"net/http"
)

func Write(w http.ResponseWriter, r *http.Request, err error) {
	status, problem := Problem(err)
	problem.Instance = new(r.URL.Path)
	if status >= http.StatusInternalServerError {
		Record(w, err)
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	_ = json.NewEncoder(w).Encode(problem)
}

// Record preserves the original error for the access log when the writer supports it.
func Record(w http.ResponseWriter, err error) {
	if recorder, ok := w.(errorRecorder); ok && err != nil {
		recorder.RecordError(err)
	}
}
