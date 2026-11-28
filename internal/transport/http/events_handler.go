package httptransport

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/service"
)

// EventService defines the event operations required by HTTP handlers.
type EventService interface {
	CreateEvent(ctx context.Context, input service.CreateEventInput) (domain.Event, error)
	GetEvent(ctx context.Context, eventID string) (domain.Event, error)
}

type createEventRequest struct {
	Name       string    `json:"name"`
	TotalStock int       `json:"total_stock"`
	StartsAt   time.Time `json:"starts_at"`
	EndsAt     time.Time `json:"ends_at"`
}

// EventHandler handles HTTP requests for /v1/events endpoints.
type EventHandler struct {
	svc    EventService
	logger *slog.Logger
}

// NewEventHandler creates an EventHandler.
func NewEventHandler(svc EventService, logger *slog.Logger) *EventHandler {
	return &EventHandler{
		svc:    svc,
		logger: logger,
	}
}

// Create handles POST /v1/events.
func (h *EventHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createEventRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, r, h.logger, http.StatusBadRequest, "INVALID_INPUT", "invalid JSON request body")
		return
	}

	ev, err := h.svc.CreateEvent(r.Context(), service.CreateEventInput{
		Name:       req.Name,
		TotalStock: req.TotalStock,
		StartsAt:   req.StartsAt,
		EndsAt:     req.EndsAt,
	})
	if err != nil {
		writeDomainError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusCreated, ev)
}

// GetByID handles GET /v1/events/{event_id}.
func (h *EventHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	eventID := r.PathValue("event_id")
	ev, err := h.svc.GetEvent(r.Context(), eventID)
	if err != nil {
		writeDomainError(w, r, h.logger, err)
		return
	}

	writeJSON(w, http.StatusOK, ev)
}
