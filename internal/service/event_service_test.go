package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/service"
)

type stubEventRepo struct {
	createFn func(ctx context.Context, ev domain.Event) (domain.Event, error)
	getFn    func(ctx context.Context, id string) (domain.Event, error)
}

func (s *stubEventRepo) CreateEvent(ctx context.Context, ev domain.Event) (domain.Event, error) {
	return s.createFn(ctx, ev)
}

func (s *stubEventRepo) GetEventByID(ctx context.Context, id string) (domain.Event, error) {
	return s.getFn(ctx, id)
}

func TestEventService_CreateEvent_Validation(t *testing.T) {
	now := time.Now().UTC()
	svc := service.NewEventService(&stubEventRepo{
		createFn: func(_ context.Context, ev domain.Event) (domain.Event, error) {
			return ev, nil
		},
	})

	cases := []struct {
		name    string
		input   service.CreateEventInput
		wantErr error
	}{
		{
			name: "empty name",
			input: service.CreateEventInput{
				Name:       "   ",
				TotalStock: 10,
				StartsAt:   now,
				EndsAt:     now.Add(time.Hour),
			},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "non-positive stock",
			input: service.CreateEventInput{
				Name:       "Concert",
				TotalStock: 0,
				StartsAt:   now,
				EndsAt:     now.Add(time.Hour),
			},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "ends_at before starts_at",
			input: service.CreateEventInput{
				Name:       "Concert",
				TotalStock: 10,
				StartsAt:   now,
				EndsAt:     now.Add(-time.Minute),
			},
			wantErr: domain.ErrInvalidInput,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateEvent(context.Background(), tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestEventService_CreateAndGet_Success(t *testing.T) {
	now := time.Now().UTC()
	var captured domain.Event
	svc := service.NewEventService(&stubEventRepo{
		createFn: func(_ context.Context, ev domain.Event) (domain.Event, error) {
			captured = ev
			return ev, nil
		},
		getFn: func(_ context.Context, id string) (domain.Event, error) {
			if id != captured.ID {
				return domain.Event{}, domain.ErrNotFound
			}
			return captured, nil
		},
	})

	ev, err := svc.CreateEvent(context.Background(), service.CreateEventInput{
		Name:       "Flash Sale",
		TotalStock: 1000,
		StartsAt:   now,
		EndsAt:     now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.ID == "" || ev.AvailableStock != 1000 {
		t.Fatalf("unexpected event created: %+v", ev)
	}

	got, err := svc.GetEvent(context.Background(), ev.ID)
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}
	if got.ID != ev.ID {
		t.Fatalf("expected ID %s, got %s", ev.ID, got.ID)
	}

	if _, err := svc.GetEvent(context.Background(), "not-a-uuid"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for bad UUID, got %v", err)
	}
}
