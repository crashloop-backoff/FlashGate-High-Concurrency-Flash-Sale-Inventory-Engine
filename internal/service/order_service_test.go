package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/service"
)

type stubOrderRepo struct {
	reserveFn func(ctx context.Context, params domain.ReserveOrderParams) (domain.ReserveOrderResult, error)
	payFn     func(ctx context.Context, orderID string) (domain.Order, error)
	cancelFn  func(ctx context.Context, orderID string) (domain.Order, error)
	expireFn  func(ctx context.Context, batchSize int) (int64, error)
}

func (s *stubOrderRepo) ReserveOrder(ctx context.Context, params domain.ReserveOrderParams) (domain.ReserveOrderResult, error) {
	return s.reserveFn(ctx, params)
}
func (s *stubOrderRepo) PayOrder(ctx context.Context, orderID string) (domain.Order, error) {
	return s.payFn(ctx, orderID)
}
func (s *stubOrderRepo) CancelOrder(ctx context.Context, orderID string) (domain.Order, error) {
	return s.cancelFn(ctx, orderID)
}
func (s *stubOrderRepo) ExpireReservations(ctx context.Context, batchSize int) (int64, error) {
	return s.expireFn(ctx, batchSize)
}

type stubOrderMetrics struct {
	results []string
	expired int64
}

func (m *stubOrderMetrics) RecordReservation(result string) {
	m.results = append(m.results, result)
}

func (m *stubOrderMetrics) AddSweeperExpired(count int64) {
	m.expired += count
}

func TestOrderService_ReserveOrder_Validation(t *testing.T) {
	svc := service.NewOrderService(&stubOrderRepo{}, 5*time.Minute, &stubOrderMetrics{})
	validEventID := "11111111-1111-4111-8111-111111111111"

	cases := []struct {
		name    string
		input   service.ReserveOrderInput
		wantErr error
	}{
		{
			name: "invalid event uuid",
			input: service.ReserveOrderInput{
				EventID:        "bad-uuid",
				UserID:         "user-1",
				Quantity:       1,
				IdempotencyKey: "key-1",
			},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "missing user_id",
			input: service.ReserveOrderInput{
				EventID:        validEventID,
				UserID:         "",
				Quantity:       1,
				IdempotencyKey: "key-1",
			},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "missing idempotency key",
			input: service.ReserveOrderInput{
				EventID:        validEventID,
				UserID:         "user-1",
				Quantity:       1,
				IdempotencyKey: " ",
			},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "quantity zero",
			input: service.ReserveOrderInput{
				EventID:        validEventID,
				UserID:         "user-1",
				Quantity:       0,
				IdempotencyKey: "key-1",
			},
			wantErr: domain.ErrInvalidQuantity,
		},
		{
			name: "quantity over max 5",
			input: service.ReserveOrderInput{
				EventID:        validEventID,
				UserID:         "user-1",
				Quantity:       6,
				IdempotencyKey: "key-1",
			},
			wantErr: domain.ErrInvalidQuantity,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.ReserveOrder(context.Background(), tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestOrderService_ReserveOrder_MetricsAndTTL(t *testing.T) {
	validEventID := "11111111-1111-4111-8111-111111111111"
	metrics := &stubOrderMetrics{}

	var callCount int
	repo := &stubOrderRepo{
		reserveFn: func(_ context.Context, params domain.ReserveOrderParams) (domain.ReserveOrderResult, error) {
			callCount++
			switch callCount {
			case 1:
				if params.OrderID == "" || params.ExpiresAt.Before(time.Now().UTC().Add(4*time.Minute)) {
					t.Fatalf("unexpected params: %+v", params)
				}
				return domain.ReserveOrderResult{
					Order:    domain.Order{ID: params.OrderID, Status: domain.OrderStatusReserved},
					Replayed: false,
				}, nil
			case 2:
				return domain.ReserveOrderResult{
					Order:    domain.Order{ID: "existing-id", Status: domain.OrderStatusReserved},
					Replayed: true,
				}, nil
			case 3:
				return domain.ReserveOrderResult{}, domain.ErrOutOfStock
			default:
				return domain.ReserveOrderResult{}, domain.ErrEventNotActive
			}
		},
	}

	svc := service.NewOrderService(repo, 5*time.Minute, metrics)
	in := service.ReserveOrderInput{
		EventID:        validEventID,
		UserID:         "user-1",
		Quantity:       2,
		IdempotencyKey: "key-1",
	}

	if _, err := svc.ReserveOrder(context.Background(), in); err != nil {
		t.Fatalf("call 1 failed: %v", err)
	}
	if res, err := svc.ReserveOrder(context.Background(), in); err != nil || !res.Replayed {
		t.Fatalf("call 2 expected replay, got res=%+v err=%v", res, err)
	}
	if _, err := svc.ReserveOrder(context.Background(), in); !errors.Is(err, domain.ErrOutOfStock) {
		t.Fatalf("call 3 expected ErrOutOfStock, got %v", err)
	}
	if _, err := svc.ReserveOrder(context.Background(), in); !errors.Is(err, domain.ErrEventNotActive) {
		t.Fatalf("call 4 expected ErrEventNotActive, got %v", err)
	}

	want := []string{"success", "idempotent_replay", "out_of_stock", "error"}
	if len(metrics.results) != len(want) {
		t.Fatalf("expected %d metrics, got %v", len(want), metrics.results)
	}
	for i := range want {
		if metrics.results[i] != want[i] {
			t.Errorf("metric[%d]: expected %q, got %q", i, want[i], metrics.results[i])
		}
	}
}
