package httptransport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/observability"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// RequestIDHeader is the HTTP header used to propagate or return request IDs.
const RequestIDHeader = "X-Request-ID"

// RequestIDFromContext retrieves the request ID stored in ctx, or an empty string if absent.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// RequestIDMiddleware ensures every request has an X-Request-ID header and stores it in context.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := strings.TrimSpace(r.Header.Get(RequestIDHeader))
		if reqID == "" {
			reqID = newRequestID()
		}
		w.Header().Set(RequestIDHeader, reqID)
		ctx := context.WithValue(r.Context(), requestIDKey, reqID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b[:])
}

type statusResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	wroteHeader  bool
}

func newStatusResponseWriter(w http.ResponseWriter) *statusResponseWriter {
	return &statusResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (w *statusResponseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = code
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// ObservabilityMiddleware records HTTP RED metrics and emits structured access logs.
func ObservabilityMiddleware(logger *slog.Logger, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			if metrics != nil {
				metrics.HTTPInflight.Inc()
				defer metrics.HTTPInflight.Dec()
			}

			rw := newStatusResponseWriter(w)
			next.ServeHTTP(rw, r)

			duration := time.Since(start)
			route := extractRoutePattern(r.Pattern)

			if metrics != nil {
				metrics.HTTPRequestsTotal.WithLabelValues(
					r.Method,
					route,
					strconv.Itoa(rw.statusCode),
				).Inc()
				metrics.HTTPRequestDuration.WithLabelValues(
					r.Method,
					route,
				).Observe(duration.Seconds())
			}

			if logger != nil {
				logger.InfoContext(r.Context(), "http request",
					slog.String("request_id", RequestIDFromContext(r.Context())),
					slog.String("method", r.Method),
					slog.String("route", route),
					slog.String("path", r.URL.Path),
					slog.Int("status", rw.statusCode),
					slog.Float64("duration_ms", float64(duration.Microseconds())/1000.0),
					slog.Int("bytes", rw.bytesWritten),
					slog.String("remote_addr", r.RemoteAddr),
				)
			}
		})
	}
}

// extractRoutePattern normalizes Go 1.22+ ServeMux r.Pattern (e.g., "GET /health" -> "/health")
// and returns "unmatched" when no route matched, preventing Prometheus label cardinality explosion.
func extractRoutePattern(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if _, path, ok := strings.Cut(pattern, " "); ok && path != "" {
		return path
	}
	return pattern
}

// RecoverMiddleware prevents unexpected panics from crashing the HTTP server.
func RecoverMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if logger != nil {
						logger.ErrorContext(r.Context(), "panic recovered",
							slog.Any("panic", rec),
							slog.String("request_id", RequestIDFromContext(r.Context())),
							slog.String("path", r.URL.Path),
						)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal server error"}`))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
