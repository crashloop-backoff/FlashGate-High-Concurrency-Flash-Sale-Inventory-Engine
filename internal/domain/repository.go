package domain

import "context"

// EventRepository defines persistence operations for flash-sale events.
type EventRepository interface {
	CreateEvent(ctx context.Context, event Event) (Event, error)
	GetEventByID(ctx context.Context, id string) (Event, error)
}

// OrderRepository defines atomic inventory and order lifecycle operations.
type OrderRepository interface {
	ReserveOrder(ctx context.Context, params ReserveOrderParams) (ReserveOrderResult, error)
	PayOrder(ctx context.Context, orderID string) (Order, error)
	CancelOrder(ctx context.Context, orderID string) (Order, error)
	ExpireReservations(ctx context.Context, batchSize int) (int64, error)
}
