package api

import (
	"net/http"

	"sportstips/internal/auth"
)

func (h *Handler) listSignals(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	sigType := r.URL.Query().Get("type")

	signals, err := h.store.GetSignals(r.Context(), claims.TenantID, sigType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server error")
		return
	}
	writeJSON(w, http.StatusOK, signals)
}
