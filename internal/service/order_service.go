package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
)

const (
	// MinOrderQuantity is the minimum tickets per reservation.
	MinOrderQuantity = 1
	// MaxOrderQuantity is the maximum tickets per reservation.
	MaxOrderQuantity = 5
	// MaxIdempotencyKeyLen matches the VARCHAR(128) schema constraint.
	MaxIdempotencyKeyLen = 128
)

// ReservationMetrics records Prometheus reservation outcomes without coupling service to Prometheus.
type ReservationMetrics interface {
	RecordReservation(result string)
}

// ReserveOrderInput holds parameters to reserve tickets for an event.
type ReserveOrderInput struct {
	EventID        string
	UserID         string
	Quantity       int
	IdempotencyKey string
}

// OrderService orchestrates ticket reservations, payments, and cancellations.
type OrderService struct {
	repo           domain.OrderRepository
	reservationTTL time.Duration
	metrics        ReservationMetrics
}

// NewOrderService creates a new OrderService.
func NewOrderService(repo domain.OrderRepository, reservationTTL time.Duration, metrics ReservationMetrics) *OrderService {
	return &OrderService{
		repo:           repo,
		reservationTTL: reservationTTL,
		metrics:        metrics,
	}
}

// ReserveOrder validates inputs, computes expires_at, invokes atomic reservation,
// and records flashgate_reservations_total metrics.
func (s *OrderService) ReserveOrder(ctx context.Context, input ReserveOrderInput) (domain.ReserveOrderResult, error) {
	eventID := strings.TrimSpace(input.EventID)
	userID := strings.TrimSpace(input.UserID)
	idemKey := strings.TrimSpace(input.IdempotencyKey)

	if !isValidUUID(eventID) || userID == "" || idemKey == "" || len(idemKey) > MaxIdempotencyKeyLen {
		return domain.ReserveOrderResult{}, domain.ErrInvalidInput
	}
	if input.Quantity < MinOrderQuantity || input.Quantity > MaxOrderQuantity {
		return domain.ReserveOrderResult{}, domain.ErrInvalidQuantity
	}

	orderID, err := newUUIDv4()
	if err != nil {
		return domain.ReserveOrderResult{}, err
	}

	res, err := s.repo.ReserveOrder(ctx, domain.ReserveOrderParams{
		OrderID:        orderID,
		EventID:        eventID,
		UserID:         userID,
		Quantity:       input.Quantity,
		IdempotencyKey: idemKey,
		ExpiresAt:      time.Now().UTC().Add(s.reservationTTL),
	})
	if err != nil {
		if s.metrics != nil {
			if errors.Is(err, domain.ErrOutOfStock) {
				s.metrics.RecordReservation("out_of_stock")
			} else {
				s.metrics.RecordReservation("error")
			}
		}
		return domain.ReserveOrderResult{}, err
	}

	if s.metrics != nil {
		if res.Replayed {
			s.metrics.RecordReservation("idempotent_replay")
		} else {
			s.metrics.RecordReservation("success")
		}
	}
	return res, nil
}

// PayOrder validates orderID and transitions a RESERVED order to PAID.
func (s *OrderService) PayOrder(ctx context.Context, orderID string) (domain.Order, error) {
	id := strings.TrimSpace(orderID)
	if !isValidUUID(id) {
		return domain.Order{}, domain.ErrInvalidInput
	}
	return s.repo.PayOrder(ctx, id)
}

// CancelOrder validates orderID and transitions a RESERVED order to CANCELLED.
func (s *OrderService) CancelOrder(ctx context.Context, orderID string) (domain.Order, error) {
	id := strings.TrimSpace(orderID)
	if !isValidUUID(id) {
		return domain.Order{}, domain.ErrInvalidInput
	}
	return s.repo.CancelOrder(ctx, id)
}
