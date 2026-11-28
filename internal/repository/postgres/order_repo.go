package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
)

// OrderRepository implements domain.OrderRepository using atomic PostgreSQL transactions.
type OrderRepository struct {
	pool *pgxpool.Pool
}

// NewOrderRepository creates a new PostgreSQL-backed OrderRepository.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getOrderByIdempotencyKey(ctx context.Context, q rowQuerier, key string) (domain.Order, bool, error) {
	const query = `
		SELECT id, event_id, user_id, quantity, status, idempotency_key, expires_at, created_at, updated_at
		FROM orders
		WHERE idempotency_key = $1
	`
	var o domain.Order
	var status string
	err := q.QueryRow(ctx, query, key).Scan(
		&o.ID,
		&o.EventID,
		&o.UserID,
		&o.Quantity,
		&status,
		&o.IdempotencyKey,
		&o.ExpiresAt,
		&o.CreatedAt,
		&o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Order{}, false, nil
		}
		return domain.Order{}, false, err
	}
	o.Status = domain.OrderStatus(status)
	return o, true, nil
}

func matchIdempotentOrder(existing domain.Order, params domain.ReserveOrderParams) (domain.ReserveOrderResult, error) {
	if existing.EventID == params.EventID &&
		existing.UserID == params.UserID &&
		existing.Quantity == params.Quantity {
		return domain.ReserveOrderResult{
			Order:    existing,
			Replayed: true,
		}, nil
	}
	return domain.ReserveOrderResult{}, domain.ErrIdempotencyConflict
}

// ReserveOrder atomically validates the event window, decrements available_stock,
// and inserts a RESERVED order while handling concurrent idempotency_key collisions.
func (r *OrderRepository) ReserveOrder(ctx context.Context, params domain.ReserveOrderParams) (domain.ReserveOrderResult, error) {
	if existing, found, err := getOrderByIdempotencyKey(ctx, r.pool, params.IdempotencyKey); err != nil {
		return domain.ReserveOrderResult{}, err
	} else if found {
		return matchIdempotentOrder(existing, params)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.ReserveOrderResult{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const decrementQuery = `
		UPDATE events
		SET available_stock = available_stock - $1, updated_at = NOW()
		WHERE id = $2
		  AND available_stock >= $1
		  AND starts_at <= NOW()
		  AND ends_at > NOW()
		RETURNING id
	`
	var updatedEventID string
	err = tx.QueryRow(ctx, decrementQuery, params.Quantity, params.EventID).Scan(&updatedEventID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ReserveOrderResult{}, r.classifyReserveFailure(ctx, tx, params.EventID)
		}
		return domain.ReserveOrderResult{}, err
	}

	const insertOrderQuery = `
		INSERT INTO orders (id, event_id, user_id, quantity, status, idempotency_key, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`
	order := domain.Order{
		ID:             params.OrderID,
		EventID:        params.EventID,
		UserID:         params.UserID,
		Quantity:       params.Quantity,
		Status:         domain.OrderStatusReserved,
		IdempotencyKey: params.IdempotencyKey,
		ExpiresAt:      params.ExpiresAt,
	}
	err = tx.QueryRow(
		ctx,
		insertOrderQuery,
		order.ID,
		order.EventID,
		order.UserID,
		order.Quantity,
		string(order.Status),
		order.IdempotencyKey,
		order.ExpiresAt,
	).Scan(&order.CreatedAt, &order.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Roll back the stock decrement before reading the committed idempotent order.
			_ = tx.Rollback(ctx)
			existing, found, lookupErr := getOrderByIdempotencyKey(ctx, r.pool, params.IdempotencyKey)
			if lookupErr != nil {
				return domain.ReserveOrderResult{}, lookupErr
			}
			if found {
				return matchIdempotentOrder(existing, params)
			}
		}
		return domain.ReserveOrderResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.ReserveOrderResult{}, err
	}
	return domain.ReserveOrderResult{
		Order:    order,
		Replayed: false,
	}, nil
}

func (r *OrderRepository) classifyReserveFailure(ctx context.Context, q rowQuerier, eventID string) error {
	const checkQuery = `
		SELECT (starts_at <= NOW() AND ends_at > NOW()) AS is_active
		FROM events
		WHERE id = $1
	`
	var isActive bool
	err := q.QueryRow(ctx, checkQuery, eventID).Scan(&isActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		return err
	}
	if !isActive {
		return domain.ErrEventNotActive
	}
	return domain.ErrOutOfStock
}

// PayOrder atomically transitions a RESERVED, non-expired order to PAID.
func (r *OrderRepository) PayOrder(ctx context.Context, orderID string) (domain.Order, error) {
	const query = `
		UPDATE orders
		SET status = $2, updated_at = NOW()
		WHERE id = $1
		  AND status = $3
		  AND expires_at > NOW()
		RETURNING id, event_id, user_id, quantity, status, idempotency_key, expires_at, created_at, updated_at
	`
	var o domain.Order
	var status string
	err := r.pool.QueryRow(
		ctx,
		query,
		orderID,
		string(domain.OrderStatusPaid),
		string(domain.OrderStatusReserved),
	).Scan(
		&o.ID,
		&o.EventID,
		&o.UserID,
		&o.Quantity,
		&status,
		&o.IdempotencyKey,
		&o.ExpiresAt,
		&o.CreatedAt,
		&o.UpdatedAt,
	)
	if err == nil {
		o.Status = domain.OrderStatus(status)
		return o, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, err
	}

	const inspectQuery = `
		SELECT status, (expires_at <= NOW()) AS is_expired
		FROM orders
		WHERE id = $1
	`
	var currentStatus string
	var isExpired bool
	inspectErr := r.pool.QueryRow(ctx, inspectQuery, orderID).Scan(&currentStatus, &isExpired)
	if inspectErr != nil {
		if errors.Is(inspectErr, pgx.ErrNoRows) {
			return domain.Order{}, domain.ErrNotFound
		}
		return domain.Order{}, inspectErr
	}
	if currentStatus == string(domain.OrderStatusExpired) || (currentStatus == string(domain.OrderStatusReserved) && isExpired) {
		return domain.Order{}, domain.ErrOrderExpired
	}
	return domain.Order{}, domain.ErrInvalidStateTransition
}

// CancelOrder atomically transitions a RESERVED order to CANCELLED and restores stock
// if and only if the status update affected exactly one row.
func (r *OrderRepository) CancelOrder(ctx context.Context, orderID string) (domain.Order, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const cancelQuery = `
		UPDATE orders
		SET status = $2, updated_at = NOW()
		WHERE id = $1
		  AND status = $3
		RETURNING id, event_id, user_id, quantity, status, idempotency_key, expires_at, created_at, updated_at
	`
	var o domain.Order
	var status string
	err = tx.QueryRow(
		ctx,
		cancelQuery,
		orderID,
		string(domain.OrderStatusCancelled),
		string(domain.OrderStatusReserved),
	).Scan(
		&o.ID,
		&o.EventID,
		&o.UserID,
		&o.Quantity,
		&status,
		&o.IdempotencyKey,
		&o.ExpiresAt,
		&o.CreatedAt,
		&o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			var existingStatus string
			inspectErr := tx.QueryRow(ctx, `SELECT status FROM orders WHERE id = $1`, orderID).Scan(&existingStatus)
			if inspectErr != nil {
				if errors.Is(inspectErr, pgx.ErrNoRows) {
					return domain.Order{}, domain.ErrNotFound
				}
				return domain.Order{}, inspectErr
			}
			return domain.Order{}, domain.ErrInvalidStateTransition
		}
		return domain.Order{}, err
	}
	o.Status = domain.OrderStatus(status)

	const restoreQuery = `
		UPDATE events
		SET available_stock = available_stock + $1, updated_at = NOW()
		WHERE id = $2
	`
	tag, err := tx.Exec(ctx, restoreQuery, o.Quantity, o.EventID)
	if err != nil {
		return domain.Order{}, err
	}
	if tag.RowsAffected() != 1 {
		return domain.Order{}, fmt.Errorf("expected 1 event row updated on cancel, got %d", tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, err
	}
	return o, nil
}

// ExpireReservations atomically marks up to batchSize expired RESERVED orders as EXPIRED
// using FOR UPDATE SKIP LOCKED and restores their aggregated quantities to events.available_stock.
func (r *OrderRepository) ExpireReservations(ctx context.Context, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 100
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const selectExpiredQuery = `
		SELECT id, event_id, quantity
		FROM orders
		WHERE status = $1 AND expires_at <= NOW()
		ORDER BY expires_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`
	rows, err := tx.Query(ctx, selectExpiredQuery, string(domain.OrderStatusReserved), batchSize)
	if err != nil {
		return 0, err
	}

	var orderIDs []string
	stockByEvent := make(map[string]int)
	for rows.Next() {
		var orderID, eventID string
		var qty int
		if scanErr := rows.Scan(&orderID, &eventID, &qty); scanErr != nil {
			rows.Close()
			return 0, scanErr
		}
		orderIDs = append(orderIDs, orderID)
		stockByEvent[eventID] += qty
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		rows.Close()
		return 0, rowsErr
	}
	rows.Close()

	if len(orderIDs) == 0 {
		return 0, nil
	}

	const expireOrdersQuery = `
		UPDATE orders
		SET status = $1, updated_at = NOW()
		WHERE id = ANY($2::uuid[])
		  AND status = $3
	`
	tag, err := tx.Exec(
		ctx,
		expireOrdersQuery,
		string(domain.OrderStatusExpired),
		orderIDs,
		string(domain.OrderStatusReserved),
	)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() != int64(len(orderIDs)) {
		return 0, fmt.Errorf("expected %d expired orders updated, got %d", len(orderIDs), tag.RowsAffected())
	}

	// Sort event IDs for deterministic row-lock acquisition order across concurrent sweepers.
	eventIDs := make([]string, 0, len(stockByEvent))
	for eventID := range stockByEvent {
		eventIDs = append(eventIDs, eventID)
	}
	sort.Strings(eventIDs)

	const restoreStockQuery = `
		UPDATE events
		SET available_stock = available_stock + $1, updated_at = NOW()
		WHERE id = $2
	`
	for _, eventID := range eventIDs {
		if _, err := tx.Exec(ctx, restoreStockQuery, stockByEvent[eventID], eventID); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
