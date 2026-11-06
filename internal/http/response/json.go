package response

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/Penlk/avito-labs/internal/http/domainerror"
)

func WriteJSON(
	w http.ResponseWriter, r *http.Request, status int,
	jsonObject any, header map[string]string,
) {
	data, err := json.Marshal(jsonObject)
	if err != nil {
		domainerror.Write(w, r, err)
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
