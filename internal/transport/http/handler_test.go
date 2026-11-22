package httptransport_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/observability"
	httptransport "github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/transport/http"
)

type stubPinger struct {
	err error
}

func (s stubPinger) Ping(_ context.Context) error {
	return s.err
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func TestHealthHandler_OK(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := observability.NewMetrics(reg)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	router := httptransport.NewRouter(
		discardLogger(),
		stubPinger{err: nil},
		metrics,
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if rec.Header().Get(httptransport.RequestIDHeader) == "" {
		t.Error("expected X-Request-ID header to be set")
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if body["status"] != "ok" || body["postgres"] != "up" || body["redis"] != "disabled" {
		t.Fatalf("unexpected health body: %+v", body)
	}
}

func TestHealthHandler_DBDown(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := observability.NewMetrics(reg)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	router := httptransport.NewRouter(
		discardLogger(),
		stubPinger{err: errors.New("connection refused")},
		metrics,
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if body["status"] != "degraded" || body["postgres"] != "down" || body["redis"] != "disabled" {
		t.Fatalf("unexpected health body: %+v", body)
	}
}

func TestObservabilityMiddleware_RecordsREDMetricsWithRoutePattern(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics, err := observability.NewMetrics(reg)
	if err != nil {
		t.Fatalf("failed to create metrics: %v", err)
	}

	router := httptransport.NewRouter(
		discardLogger(),
		stubPinger{err: nil},
		metrics,
		promhttp.HandlerFor(reg, promhttp.HandlerOpts{}),
	)

	// Matched route
	reqOK := httptest.NewRequest(http.MethodGet, "/health", nil)
	reqOK.Header.Set(httptransport.RequestIDHeader, "trace-123")
	recOK := httptest.NewRecorder()
	router.ServeHTTP(recOK, reqOK)

	if got := recOK.Header().Get(httptransport.RequestIDHeader); got != "trace-123" {
		t.Errorf("expected preserved X-Request-ID trace-123, got %q", got)
	}

	// Unmatched route should use "unmatched" label, not raw path
	reqNotFound := httptest.NewRequest(http.MethodGet, "/does-not-exist/abc-123", nil)
	recNotFound := httptest.NewRecorder()
	router.ServeHTTP(recNotFound, reqNotFound)

	if recNotFound.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recNotFound.Code)
	}

	if got := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "/health", "200")); got != 1 {
		t.Errorf("expected flashgate_http_requests_total{GET,/health,200} = 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.HTTPRequestsTotal.WithLabelValues("GET", "unmatched", "404")); got != 1 {
		t.Errorf("expected flashgate_http_requests_total{GET,unmatched,404} = 1, got %v", got)
	}
	if got := testutil.ToFloat64(metrics.HTTPInflight); got != 0 {
		t.Errorf("expected flashgate_http_inflight_requests = 0 after completion, got %v", got)
	}
}
