package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"sportstips/internal/auth"
	"sportstips/internal/store"
)

type Handler struct {
	store     *store.Store
	jwtSecret string
}

func NewHandler(s *store.Store, jwtSecret string) *Handler {
	return &Handler{store: s, jwtSecret: jwtSecret}
}

func (h *Handler) Router() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	r.Post("/auth/register", h.register)
	r.Post("/auth/login", h.login)

	r.Group(func(r chi.Router) {
		r.Use(auth.Middleware(h.jwtSecret))
		r.Get("/matches", h.listMatches)
		r.Get("/matches/{id}/odds", h.getMatchOdds)
		r.Get("/matches/{id}/signals", h.getMatchSignals)
		r.Get("/signals", h.listSignals)
		r.Patch("/preferences", h.updatePreferences)
	})

	return r
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
