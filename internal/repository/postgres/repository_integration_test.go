package postgres_test

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/repository/postgres"
)

func mustTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping postgres integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to TEST_DATABASE_URL: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping TEST_DATABASE_URL: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newTestUUID(t *testing.T) string {
	t.Helper()
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("failed to generate uuid: %v", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func createTestEvent(t *testing.T, pool *pgxpool.Pool, repo *postgres.EventRepository, totalStock int) domain.Event {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	ev, err := repo.CreateEvent(ctx, domain.Event{
		ID:             newTestUUID(t),
		Name:           "Integration Test Event",
		TotalStock:     totalStock,
		AvailableStock: totalStock,
		StartsAt:       now.Add(-1 * time.Minute),
		EndsAt:         now.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to create test event: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM orders WHERE event_id = $1`, ev.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM events WHERE id = $1`, ev.ID)
	})
	return ev
}

func TestPostgres_Concurrency_NoOverselling(t *testing.T) {
	pool := mustTestPool(t)
	eventRepo := postgres.NewEventRepository(pool)
	orderRepo := postgres.NewOrderRepository(pool)

	ev := createTestEvent(t, pool, eventRepo, 10)

	const totalWorkers = 50
	var successCount atomic.Int32
	var outOfStockCount atomic.Int32
	var unexpectedErrCount atomic.Int32

	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(totalWorkers)

	for i := 0; i < totalWorkers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-startGate
			_, err := orderRepo.ReserveOrder(context.Background(), domain.ReserveOrderParams{
				OrderID:        newTestUUID(t),
				EventID:        ev.ID,
				UserID:         fmt.Sprintf("user-%d", i),
				Quantity:       1,
				IdempotencyKey: fmt.Sprintf("conc-%s-%d", ev.ID, i),
				ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
			})
			switch {
			case err == nil:
				successCount.Add(1)
			case errors.Is(err, domain.ErrOutOfStock):
				outOfStockCount.Add(1)
			default:
				unexpectedErrCount.Add(1)
				t.Errorf("worker %d unexpected error: %v", i, err)
			}
		}()
	}

	close(startGate)
	wg.Wait()

	if unexpectedErrCount.Load() != 0 {
		t.Fatalf("expected 0 unexpected errors, got %d", unexpectedErrCount.Load())
	}
	if successCount.Load() != 10 {
		t.Fatalf("expected exactly 10 successful reservations, got %d", successCount.Load())
	}
	if outOfStockCount.Load() != 40 {
		t.Fatalf("expected exactly 40 ErrOutOfStock responses, got %d", outOfStockCount.Load())
	}

	gotEvent, err := eventRepo.GetEventByID(context.Background(), ev.ID)
	if err != nil {
		t.Fatalf("GetEventByID failed: %v", err)
	}
	if gotEvent.AvailableStock != 0 {
		t.Fatalf("expected available_stock == 0, got %d", gotEvent.AvailableStock)
	}

	var reservedCount int
	err = pool.QueryRow(
		context.Background(),
		`SELECT COUNT(*) FROM orders WHERE event_id = $1 AND status = 'RESERVED'`,
		ev.ID,
	).Scan(&reservedCount)
	if err != nil {
		t.Fatalf("failed to count reserved orders: %v", err)
	}
	if reservedCount != 10 {
		t.Fatalf("expected 10 RESERVED orders, got %d", reservedCount)
	}
}

func TestPostgres_IdempotencyReplay_Concurrent(t *testing.T) {
	pool := mustTestPool(t)
	eventRepo := postgres.NewEventRepository(pool)
	orderRepo := postgres.NewOrderRepository(pool)

	ev := createTestEvent(t, pool, eventRepo, 10)
	sharedKey := fmt.Sprintf("idem-%s", ev.ID)

	const workers = 10
	orderIDs := make([]string, workers)
	startGate := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-startGate
			res, err := orderRepo.ReserveOrder(context.Background(), domain.ReserveOrderParams{
				OrderID:        newTestUUID(t),
				EventID:        ev.ID,
				UserID:         "user-idem",
				Quantity:       2,
				IdempotencyKey: sharedKey,
				ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
			})
			if err != nil {
				t.Errorf("worker %d failed: %v", i, err)
				return
			}
			orderIDs[i] = res.Order.ID
		}()
	}

	close(startGate)
	wg.Wait()

	firstID := orderIDs[0]
	if firstID == "" {
		t.Fatalf("expected non-empty order ID")
	}
	for i, id := range orderIDs {
		if id != firstID {
			t.Fatalf("worker %d got order ID %q, expected %q", i, id, firstID)
		}
	}

	gotEvent, err := eventRepo.GetEventByID(context.Background(), ev.ID)
	if err != nil {
		t.Fatalf("GetEventByID failed: %v", err)
	}
	if gotEvent.AvailableStock != 8 {
		t.Fatalf("expected available_stock == 8 (decremented once by 2), got %d", gotEvent.AvailableStock)
	}

	// Reusing the same Idempotency-Key with a different quantity must return ErrIdempotencyConflict.
	_, err = orderRepo.ReserveOrder(context.Background(), domain.ReserveOrderParams{
		OrderID:        newTestUUID(t),
		EventID:        ev.ID,
		UserID:         "user-idem",
		Quantity:       3,
		IdempotencyKey: sharedKey,
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
	})
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("expected ErrIdempotencyConflict, got %v", err)
	}
}

func TestPostgres_SweeperAndCancel_NoDoubleRestore(t *testing.T) {
	pool := mustTestPool(t)
	eventRepo := postgres.NewEventRepository(pool)
	orderRepo := postgres.NewOrderRepository(pool)

	ev := createTestEvent(t, pool, eventRepo, 10)
	ctx := context.Background()

	// Order 1: expired reservation (qty 3)
	expiredRes, err := orderRepo.ReserveOrder(ctx, domain.ReserveOrderParams{
		OrderID:        newTestUUID(t),
		EventID:        ev.ID,
		UserID:         "user-exp",
		Quantity:       3,
		IdempotencyKey: fmt.Sprintf("exp-%s", ev.ID),
		ExpiresAt:      time.Now().UTC().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to reserve expired order: %v", err)
	}

	// Order 2: active reservation to be cancelled (qty 2)
	cancelRes, err := orderRepo.ReserveOrder(ctx, domain.ReserveOrderParams{
		OrderID:        newTestUUID(t),
		EventID:        ev.ID,
		UserID:         "user-cancel",
		Quantity:       2,
		IdempotencyKey: fmt.Sprintf("cancel-%s", ev.ID),
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to reserve cancellable order: %v", err)
	}

	// Cancel Order 2
	cancelled, err := orderRepo.CancelOrder(ctx, cancelRes.Order.ID)
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}
	if cancelled.Status != domain.OrderStatusCancelled {
		t.Fatalf("expected status CANCELLED, got %s", cancelled.Status)
	}

	// Second cancel must fail with ErrInvalidStateTransition and NOT restore stock again
	if _, err := orderRepo.CancelOrder(ctx, cancelRes.Order.ID); !errors.Is(err, domain.ErrInvalidStateTransition) {
		t.Fatalf("expected ErrInvalidStateTransition on second cancel, got %v", err)
	}

	// Sweeper expires Order 1
	expiredCount, err := orderRepo.ExpireReservations(ctx, 100)
	if err != nil {
		t.Fatalf("ExpireReservations failed: %v", err)
	}
	if expiredCount < 1 {
		t.Fatalf("expected at least 1 expired order, got %d", expiredCount)
	}

	// Pay on expired Order 1 must fail with ErrOrderExpired
	if _, err := orderRepo.PayOrder(ctx, expiredRes.Order.ID); !errors.Is(err, domain.ErrOrderExpired) {
		t.Fatalf("expected ErrOrderExpired on PayOrder, got %v", err)
	}

	// Verify full stock restored accurately (10 - 3 - 2 + 2 + 3 = 10)
	gotEvent, err := eventRepo.GetEventByID(ctx, ev.ID)
	if err != nil {
		t.Fatalf("GetEventByID failed: %v", err)
	}
	if gotEvent.AvailableStock != 10 {
		t.Fatalf("expected available_stock == 10 after cancel + sweep, got %d", gotEvent.AvailableStock)
	}
}
