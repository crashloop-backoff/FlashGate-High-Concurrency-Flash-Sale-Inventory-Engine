package domain

import "time"

// OrderStatus represents the lifecycle state of a ticket reservation order.
type OrderStatus string

const (
	OrderStatusReserved  OrderStatus = "RESERVED"
	OrderStatusPaid      OrderStatus = "PAID"
	OrderStatusCancelled OrderStatus = "CANCELLED"
	OrderStatusExpired   OrderStatus = "EXPIRED"
)

// Event represents a flash-sale event with bounded ticket inventory.
type Event struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	TotalStock     int       `json:"total_stock"`
	AvailableStock int       `json:"available_stock"`
	StartsAt       time.Time `json:"starts_at"`
	EndsAt         time.Time `json:"ends_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Order represents a ticket reservation or its terminal state.
type Order struct {
	ID             string      `json:"id"`
	EventID        string      `json:"event_id"`
	UserID         string      `json:"user_id"`
	Quantity       int         `json:"quantity"`
	Status         OrderStatus `json:"status"`
	IdempotencyKey string      `json:"idempotency_key"`
	ExpiresAt      time.Time   `json:"expires_at"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

// ReserveOrderParams contains validated inputs required to atomically reserve tickets.
type ReserveOrderParams struct {
	OrderID        string
	EventID        string
	UserID         string
	Quantity       int
	IdempotencyKey string
	ExpiresAt      time.Time
}

// ReserveOrderResult holds the persisted order and whether it was an idempotent replay.
type ReserveOrderResult struct {
	Order    Order
	Replayed bool
}
