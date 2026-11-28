package httptransport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/service"
)

const (
	headerUserID         = "X-User-ID"
	headerIdempotencyKey = "Idempotency-Key"
)

// OrderService defines the order operations required by HTTP handlers.
type OrderService interface {
	ReserveOrder(ctx context.Context, input service.ReserveOrderInput) (domain.ReserveOrderResult, error)
	PayOrder(ctx context.Context, orderID string) (domain.Order, error)
	CancelOrder(ctx context.Context, orderID string) (domain.Order, error)
}

type createOrderRequest struct {
	EventID  string `json:"event_id"`
	Quantity int    `json:"quantity"`
}

// OrderHandler handles HTTP requests for /v1/orders endpoints.
type OrderHandler struct {
	svc    OrderService
	logger *slog.Logger
}

// NewOrderHandler creates an OrderHandler.
func NewOrderHandler(svc OrderService, logger *slog.Logger) *OrderHandler {
	return &OrderHandler{
		svc:    svc,
		logger: logger,
	}
}

// Reserve handles POST /v1/orders.
func (h *OrderHandler) Reserve(w http.ResponseWriter, r *http.Request) {
	userID := strings.TrimSpace(r.Header.Get(headerUserID))
	idemKey := strings.TrimSpace(r.Header.Get(headerIdempotencyKey))
	if userID == "" || idemKey == "" {
		writeError(w, r, h.logger, http.StatusBadRequest, "INVALID_INPUT", "X-User-ID and Idempotency-Key headers are required")
		return
	}

	var req createOrderRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, r, h.logger, http.StatusBadRequest, "INVALID_INPUT", "invalid JSON request body")
		return
	}

	res, err := h.svc.ReserveOrder(r.Context(), service.ReserveOrderInput{
		EventID:        req.EventID,
		UserID:         userID,
		Quantity:       req.Quantity,
		IdempotencyKey: idemKey,
	})
	if err != nil {
		writeDomainError(w, r, h.logger, err)
		return
	}

	status := http.StatusCreated
	if res.Replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, res.Order)
}

// Pay handles POST /v1/orders/{order_id}/pay.
func (h *OrderHandler) Pay(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("order_id")
	order, err := h.svc.PayOrder(r.Context(), orderID)
	if err != nil {
		writeDomainError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, order)
}

// Cancel handles POST /v1/orders/{order_id}/cancel.
func (h *OrderHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("order_id")
	order, err := h.svc.CancelOrder(r.Context(), orderID)
	if err != nil {
		writeDomainError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, order)
}
