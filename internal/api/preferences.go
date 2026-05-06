package api

import (
	"encoding/json"
	"net/http"

	"sportstips/internal/auth"
	"sportstips/internal/store"
)

type preferencesRequest struct {
	MinArbProfit *float64 `json:"min_arb_profit"`
	MinValueEdge *float64 `json:"min_value_edge"`
	TelegramID   *string  `json:"alert_telegram_id"`
	Email        *string  `json:"alert_email"`
	Bookmakers   []string `json:"bookmakers"`
}

func (h *Handler) updatePreferences(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())

	var req preferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}

	current, err := h.store.GetPreferences(r.Context(), claims.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "server error")
		return
	}

	if req.MinArbProfit != nil {
		current.MinArbProfit = *req.MinArbProfit
	}
	if req.MinValueEdge != nil {
		current.MinValueEdge = *req.MinValueEdge
	}
	if req.TelegramID != nil {
		current.TelegramID = req.TelegramID
	}
	if req.Email != nil {
		current.Email = req.Email
	}
	if req.Bookmakers != nil {
		current.Bookmakers = req.Bookmakers
	}

	if err := h.store.UpdatePreferences(r.Context(), store.TenantPreferences{
		TenantID:     current.TenantID,
		MinArbProfit: current.MinArbProfit,
		MinValueEdge: current.MinValueEdge,
		TelegramID:   current.TelegramID,
		Email:        current.Email,
		Bookmakers:   current.Bookmakers,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "server error")
		return
	}

	writeJSON(w, http.StatusOK, current)
}
