package domain

import "errors"

var (
	// ErrNotFound indicates the requested event or order does not exist.
	ErrNotFound = errors.New("resource not found")
	// ErrOutOfStock indicates insufficient available_stock for the requested quantity.
	ErrOutOfStock = errors.New("insufficient available stock")
	// ErrEventNotActive indicates the current time is outside [starts_at, ends_at).
	ErrEventNotActive = errors.New("event is not active")
	// ErrInvalidQuantity indicates quantity is outside the allowed 1..5 range.
	ErrInvalidQuantity = errors.New("quantity must be between 1 and 5")
	// ErrOrderExpired indicates the reservation has already expired.
	ErrOrderExpired = errors.New("order reservation has expired")
	// ErrInvalidStateTransition indicates an order cannot transition from its current status.
	ErrInvalidStateTransition = errors.New("invalid order state transition")
	// ErrIdempotencyConflict indicates an idempotency key was reused with a different payload.
	ErrIdempotencyConflict = errors.New("idempotency key conflict")
	// ErrInvalidInput indicates malformed or missing request parameters.
	ErrInvalidInput = errors.New("invalid input")
)
