package api

import (
	"context"
	"net/http"

	"sportstips/internal/alerts"
)

func (h *Handler) resendAlerts(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	tenants, err := h.store.GetAllTenants(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tenants")
		return
	}

	total := 0
	for _, tenant := range tenants {
		prefs, err := h.store.GetPreferences(ctx, tenant.ID)
		if err != nil || prefs.TelegramID == nil || *prefs.TelegramID == "" {
			continue
		}

		sigs, err := h.store.GetSignals(ctx, tenant.ID, "")
		if err != nil {
			continue
		}

		for _, sig := range sigs {
			matchLabel := sig.MatchID
			if m, err := h.store.GetMatchByID(ctx, sig.MatchID); err == nil {
				matchLabel = m.HomeTeam + " vs " + m.AwayTeam
			}
			msg := alerts.FormatSignal(sig.Type, sig.Market, matchLabel, sig.Data)
			h.alerter.Send(*prefs.TelegramID, msg) //nolint:errcheck
			total++
		}
	}

	writeJSON(w, http.StatusOK, map[string]int{"sent": total})
}
