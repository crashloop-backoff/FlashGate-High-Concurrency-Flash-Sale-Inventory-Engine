package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
)

const defaultSweeperBatchSize = 100

// SweeperMetrics records expired reservation counts without coupling service to Prometheus.
type SweeperMetrics interface {
	AddSweeperExpired(count int64)
}

// Sweeper periodically expires overdue RESERVED orders and restores their stock.
type Sweeper struct {
	repo      domain.OrderRepository
	interval  time.Duration
	batchSize int
	metrics   SweeperMetrics
	logger    *slog.Logger
}

// NewSweeper creates a background Sweeper.
func NewSweeper(
	repo domain.OrderRepository,
	interval time.Duration,
	batchSize int,
	metrics SweeperMetrics,
	logger *slog.Logger,
) *Sweeper {
	if batchSize <= 0 {
		batchSize = defaultSweeperBatchSize
	}
	return &Sweeper{
		repo:      repo,
		interval:  interval,
		batchSize: batchSize,
		metrics:   metrics,
		logger:    logger,
	}
}

// Run executes the sweeper loop on a ticker until ctx is cancelled.
func (s *Sweeper) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.RunOnce(ctx)
		}
	}
}

// RunOnce executes a single expiration sweep pass.
func (s *Sweeper) RunOnce(ctx context.Context) {
	sweepCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	expiredCount, err := s.repo.ExpireReservations(sweepCtx, s.batchSize)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "sweeper failed to expire reservations", slog.String("error", err.Error()))
		}
		return
	}

	if expiredCount > 0 {
		if s.metrics != nil {
			s.metrics.AddSweeperExpired(expiredCount)
		}
		if s.logger != nil {
			s.logger.InfoContext(ctx, "sweeper expired reservations", slog.Int64("count", expiredCount))
		}
	}
}
