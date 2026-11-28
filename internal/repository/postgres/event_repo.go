package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
)

// EventRepository implements domain.EventRepository using PostgreSQL.
type EventRepository struct {
	pool *pgxpool.Pool
}

// NewEventRepository creates a new PostgreSQL-backed EventRepository.
func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

// CreateEvent inserts a new flash-sale event and returns the persisted record.
func (r *EventRepository) CreateEvent(ctx context.Context, event domain.Event) (domain.Event, error) {
	const query = `
		INSERT INTO events (id, name, total_stock, available_stock, starts_at, ends_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING created_at, updated_at
	`
	err := r.pool.QueryRow(
		ctx,
		query,
		event.ID,
		event.Name,
		event.TotalStock,
		event.AvailableStock,
		event.StartsAt,
		event.EndsAt,
	).Scan(&event.CreatedAt, &event.UpdatedAt)
	if err != nil {
		return domain.Event{}, err
	}
	return event, nil
}

// GetEventByID fetches an event by its UUID.
func (r *EventRepository) GetEventByID(ctx context.Context, id string) (domain.Event, error) {
	const query = `
		SELECT id, name, total_stock, available_stock, starts_at, ends_at, created_at, updated_at
		FROM events
		WHERE id = $1
	`
	var ev domain.Event
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&ev.ID,
		&ev.Name,
		&ev.TotalStock,
		&ev.AvailableStock,
		&ev.StartsAt,
		&ev.EndsAt,
		&ev.CreatedAt,
		&ev.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Event{}, domain.ErrNotFound
		}
		return domain.Event{}, err
	}
	return ev, nil
}
