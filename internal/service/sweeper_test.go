package service_test

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/service"
)

func TestSweeper_RunOnceAndCancel(t *testing.T) {
	var runs atomic.Int32
	metrics := &stubOrderMetrics{}
	repo := &stubOrderRepo{
		expireFn: func(_ context.Context, batchSize int) (int64, error) {
			if batchSize != 50 {
				t.Errorf("expected batchSize 50, got %d", batchSize)
			}
			runs.Add(1)
			return 3, nil
		},
	}

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	sweeper := service.NewSweeper(repo, 15*time.Millisecond, 50, metrics, logger)

	sweeper.RunOnce(context.Background())
	if metrics.expired != 3 {
		t.Fatalf("expected 3 expired metrics, got %d", metrics.expired)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sweeper.Run(ctx)
		close(done)
	}()

	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sweeper did not stop cleanly after context cancellation")
	}

	if runs.Load() < 2 {
		t.Fatalf("expected sweeper to run at least twice, got %d", runs.Load())
	}
}
