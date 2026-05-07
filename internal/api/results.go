package api

import (
	"net/http"
)

// POST /admin/results/sync — triggers immediate result ingestion.
// No auth in Phase 2 (admin-only, internal use).
func (h *Handler) syncResults(w http.ResponseWriter, r *http.Request) {
	if err := h.ingester.Run(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "sync failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
