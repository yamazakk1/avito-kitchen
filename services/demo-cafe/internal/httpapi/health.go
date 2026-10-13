package httpapi

import "net/http"

type healthResponse struct {
	Status string `json:"status"`
}

func (h handler) live(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(r.Context(), w, http.StatusOK, healthResponse{Status: "ok"})
}
