package httptransport

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
)

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, status int, code, message string) {
	reqID := RequestIDFromContext(r.Context())
	if status >= http.StatusInternalServerError && logger != nil {
		logger.ErrorContext(r.Context(), "internal server error",
			slog.String("request_id", reqID),
			slog.String("code", code),
			slog.String("message", message),
		)
	}
	writeJSON(w, status, errorResponse{
		Error: errorBody{
			Code:      code,
			Message:   message,
			RequestID: reqID,
		},
	})
}

func writeDomainError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(w, r, logger, http.StatusBadRequest, "INVALID_INPUT", err.Error())
	case errors.Is(err, domain.ErrInvalidQuantity):
		writeError(w, r, logger, http.StatusBadRequest, "INVALID_QUANTITY", err.Error())
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, r, logger, http.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, domain.ErrOutOfStock):
		writeError(w, r, logger, http.StatusConflict, "OUT_OF_STOCK", err.Error())
	case errors.Is(err, domain.ErrEventNotActive):
		writeError(w, r, logger, http.StatusConflict, "EVENT_NOT_ACTIVE", err.Error())
	case errors.Is(err, domain.ErrOrderExpired):
		writeError(w, r, logger, http.StatusConflict, "ORDER_EXPIRED", err.Error())
	case errors.Is(err, domain.ErrInvalidStateTransition):
		writeError(w, r, logger, http.StatusConflict, "INVALID_STATE_TRANSITION", err.Error())
	case errors.Is(err, domain.ErrIdempotencyConflict):
		writeError(w, r, logger, http.StatusConflict, "IDEMPOTENCY_CONFLICT", err.Error())
	default:
		if logger != nil {
			logger.ErrorContext(r.Context(), "unhandled error",
				slog.String("request_id", RequestIDFromContext(r.Context())),
				slog.String("error", err.Error()),
			)
		}
		writeError(w, r, logger, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
	}
}
