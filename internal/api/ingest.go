package api

import "net/http"

func (h *Handler) triggerIngest(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}
	go h.scheduler.Trigger(r.Context())
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
}
