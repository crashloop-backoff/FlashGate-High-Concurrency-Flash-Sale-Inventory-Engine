package service

import (
	"context"
	"strings"
	"time"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
)

// CreateEventInput holds parameters to create a new flash-sale event.
type CreateEventInput struct {
	Name       string
	TotalStock int
	StartsAt   time.Time
	EndsAt     time.Time
}

// EventService handles business validation and orchestration for events.
type EventService struct {
	repo domain.EventRepository
}

// NewEventService creates a new EventService.
func NewEventService(repo domain.EventRepository) *EventService {
	return &EventService{repo: repo}
}

// CreateEvent validates inputs, assigns a UUID v4, and persists the event.
func (s *EventService) CreateEvent(ctx context.Context, input CreateEventInput) (domain.Event, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" || input.TotalStock <= 0 || input.StartsAt.IsZero() || input.EndsAt.IsZero() {
		return domain.Event{}, domain.ErrInvalidInput
	}
	if !input.EndsAt.After(input.StartsAt) {
		return domain.Event{}, domain.ErrInvalidInput
	}

	id, err := newUUIDv4()
	if err != nil {
		return domain.Event{}, err
	}

	ev := domain.Event{
		ID:             id,
		Name:           name,
		TotalStock:     input.TotalStock,
		AvailableStock: input.TotalStock,
		StartsAt:       input.StartsAt.UTC(),
		EndsAt:         input.EndsAt.UTC(),
	}
	return s.repo.CreateEvent(ctx, ev)
}

// GetEvent validates eventID and retrieves the event.
func (s *EventService) GetEvent(ctx context.Context, eventID string) (domain.Event, error) {
	id := strings.TrimSpace(eventID)
	if !isValidUUID(id) {
		return domain.Event{}, domain.ErrInvalidInput
	}
	return s.repo.GetEventByID(ctx, id)
}
