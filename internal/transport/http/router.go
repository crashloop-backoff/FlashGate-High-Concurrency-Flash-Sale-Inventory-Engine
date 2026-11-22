package httptransport

import (
	"log/slog"
	"net/http"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/observability"
)

// NewRouter wires HTTP endpoints and middlewares using Go stdlib net/http.ServeMux.
func NewRouter(
	logger *slog.Logger,
	pinger Pinger,
	metrics *observability.Metrics,
	metricsHandler http.Handler,
) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", NewHealthHandler(pinger, logger))
	if metricsHandler != nil {
		mux.Handle("GET /metrics", metricsHandler)
	}

	var handler http.Handler = mux
	handler = ObservabilityMiddleware(logger, metrics)(handler)
	handler = RecoverMiddleware(logger)(handler)
	handler = RequestIDMiddleware(handler)
	return handler
}
