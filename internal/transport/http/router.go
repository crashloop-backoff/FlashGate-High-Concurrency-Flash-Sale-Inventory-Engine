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
	eventSvc EventService,
	orderSvc OrderService,
) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /health", NewHealthHandler(pinger, logger))
	if metricsHandler != nil {
		mux.Handle("GET /metrics", metricsHandler)
	}

	if eventSvc != nil {
		eventHandler := NewEventHandler(eventSvc, logger)
		mux.HandleFunc("POST /v1/events", eventHandler.Create)
		mux.HandleFunc("GET /v1/events/{event_id}", eventHandler.GetByID)
	}

	if orderSvc != nil {
		orderHandler := NewOrderHandler(orderSvc, logger)
		mux.HandleFunc("POST /v1/orders", orderHandler.Reserve)
		mux.HandleFunc("POST /v1/orders/{order_id}/pay", orderHandler.Pay)
		mux.HandleFunc("POST /v1/orders/{order_id}/cancel", orderHandler.Cancel)
	}

	var handler http.Handler = mux
	handler = ObservabilityMiddleware(logger, metrics)(handler)
	handler = RecoverMiddleware(logger)(handler)
	handler = RequestIDMiddleware(handler)
	return handler
}
