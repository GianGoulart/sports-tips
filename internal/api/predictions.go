package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"sportstips/internal/auth"
)

func (h *Handler) getPrediction(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	matchID := chi.URLParam(r, "match_id")
	pred, err := h.pred.Predict(r.Context(), matchID)
	if err != nil {
		writeError(w, http.StatusNotFound, "prediction not available")
		return
	}
	writeJSON(w, http.StatusOK, pred)
}
