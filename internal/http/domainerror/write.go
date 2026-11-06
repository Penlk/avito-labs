package domainerror

import (
	"encoding/json"
	"log"
	"net/http"
)

func Write(w http.ResponseWriter, r *http.Request, err error) {
	status, problem := Problem(err)
	problem.Instance = new(r.URL.Path)
	if problem.Status >= http.StatusInternalServerError {
		log.Printf("handle %s %s: %v", r.Method, r.URL.Path, err)
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(problem); err != nil {
		log.Printf("write problem response: %v", err)
	}
}
