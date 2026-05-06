package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"sportstips/internal/auth"
)

func (h *Handler) listMatches(w http.ResponseWriter, r *http.Request) {
	matches, err := h.store.GetActiveMatches(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server error")
		return
	}
	writeJSON(w, http.StatusOK, matches)
}

func (h *Handler) getMatchOdds(w http.ResponseWriter, r *http.Request) {
	matchID := chi.URLParam(r, "id")
	odds, err := h.store.GetLatestOddsByMatch(r.Context(), matchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server error")
		return
	}
	writeJSON(w, http.StatusOK, odds)
}

func (h *Handler) getMatchSignals(w http.ResponseWriter, r *http.Request) {
	matchID := chi.URLParam(r, "id")
	claims := auth.ClaimsFromContext(r.Context())

	signals, err := h.store.GetSignalsByMatch(r.Context(), claims.TenantID, matchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server error")
		return
	}
	writeJSON(w, http.StatusOK, signals)
}
