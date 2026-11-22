package httptransport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

// Pinger checks database connectivity.
type Pinger interface {
	Ping(ctx context.Context) error
}

type healthResponse struct {
	Status   string `json:"status"`
	Postgres string `json:"postgres"`
	Redis    string `json:"redis"`
}

// HealthHandler handles GET /health requests by pinging PostgreSQL.
type HealthHandler struct {
	pinger Pinger
	logger *slog.Logger
}

// NewHealthHandler constructs a HealthHandler.
func NewHealthHandler(pinger Pinger, logger *slog.Logger) *HealthHandler {
	return &HealthHandler{
		pinger: pinger,
		logger: logger,
	}
}

// ServeHTTP responds with 200 OK when PostgreSQL is reachable, or 503 Service Unavailable otherwise.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "application/json")

	if h.pinger == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(healthResponse{
			Status:   "degraded",
			Postgres: "down",
			Redis:    "disabled",
		})
		return
	}

	if err := h.pinger.Ping(ctx); err != nil {
		if h.logger != nil {
			h.logger.WarnContext(r.Context(), "health check failed",
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.String("error", err.Error()),
			)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(healthResponse{
			Status:   "degraded",
			Postgres: "down",
			Redis:    "disabled",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:   "ok",
		Postgres: "up",
		Redis:    "disabled",
	})
}
