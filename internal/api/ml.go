package api

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

var mlClient = &http.Client{Timeout: 10 * time.Second}

func (h *Handler) triggerML(w http.ResponseWriter, r *http.Request) {
	if h.mlServiceURL == "" {
		writeError(w, http.StatusServiceUnavailable, "ML service not configured")
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, h.mlServiceURL+"/run", nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build request")
		return
	}
	if h.mlSecret != "" {
		req.Header.Set("X-ML-Secret", h.mlSecret)
	}

	resp, err := mlClient.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("ML service unreachable: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
