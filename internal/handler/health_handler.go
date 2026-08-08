package handler

import (
	"context"
	"net/http"

	"github.com/cymonevo/go_template/pkg/response"
	"github.com/go-chi/chi/v5"
)

// Pinger is implemented by any dependency whose health can be checked.
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthHandler exposes liveness and readiness probes.
type HealthHandler struct {
	checks map[string]Pinger
}

// NewHealthHandler builds a HealthHandler with named dependency checks.
func NewHealthHandler(checks map[string]Pinger) *HealthHandler {
	return &HealthHandler{checks: checks}
}

func (h *HealthHandler) Register(r chi.Router) {
	r.Get("/healthz", h.Live)
	r.Get("/readyz", h.Ready)
}

// Live reports that the process is running.
func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	response.OK(w, map[string]string{"status": "ok"})
}

// Ready verifies all dependencies are reachable.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	statuses := make(map[string]string, len(h.checks))
	healthy := true
	for name, dep := range h.checks {
		if err := dep.Ping(r.Context()); err != nil {
			statuses[name] = "down"
			healthy = false
			continue
		}
		statuses[name] = "up"
	}

	if !healthy {
		response.JSON(w, http.StatusServiceUnavailable, response.Envelope{
			Success: false,
			Data:    statuses,
		})
		return
	}
	response.OK(w, statuses)
}
