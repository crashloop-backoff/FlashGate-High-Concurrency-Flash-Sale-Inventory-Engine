package httptransport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/domain"
	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/observability"
	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/service"
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
		nil,
		nil,
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
		nil,
		nil,
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
		nil,
		nil,
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

type stubEventService struct {
	createFn func(ctx context.Context, input service.CreateEventInput) (domain.Event, error)
	getFn    func(ctx context.Context, eventID string) (domain.Event, error)
}

func (s *stubEventService) CreateEvent(ctx context.Context, input service.CreateEventInput) (domain.Event, error) {
	return s.createFn(ctx, input)
}

func (s *stubEventService) GetEvent(ctx context.Context, eventID string) (domain.Event, error) {
	return s.getFn(ctx, eventID)
}

type stubOrderService struct {
	reserveFn func(ctx context.Context, input service.ReserveOrderInput) (domain.ReserveOrderResult, error)
	payFn     func(ctx context.Context, orderID string) (domain.Order, error)
	cancelFn  func(ctx context.Context, orderID string) (domain.Order, error)
}

func (s *stubOrderService) ReserveOrder(ctx context.Context, input service.ReserveOrderInput) (domain.ReserveOrderResult, error) {
	return s.reserveFn(ctx, input)
}

func (s *stubOrderService) PayOrder(ctx context.Context, orderID string) (domain.Order, error) {
	return s.payFn(ctx, orderID)
}

func (s *stubOrderService) CancelOrder(ctx context.Context, orderID string) (domain.Order, error) {
	return s.cancelFn(ctx, orderID)
}

func TestEventHandlers_CreateAndGet(t *testing.T) {
	now := time.Now().UTC()
	eventSvc := &stubEventService{
		createFn: func(_ context.Context, input service.CreateEventInput) (domain.Event, error) {
			return domain.Event{
				ID:             "11111111-1111-4111-8111-111111111111",
				Name:           input.Name,
				TotalStock:     input.TotalStock,
				AvailableStock: input.TotalStock,
				StartsAt:       input.StartsAt,
				EndsAt:         input.EndsAt,
			}, nil
		},
		getFn: func(_ context.Context, eventID string) (domain.Event, error) {
			if eventID == "missing" {
				return domain.Event{}, domain.ErrNotFound
			}
			return domain.Event{ID: eventID, Name: "Test", TotalStock: 100, AvailableStock: 99}, nil
		},
	}

	router := httptransport.NewRouter(discardLogger(), stubPinger{}, nil, nil, eventSvc, nil)

	body, _ := json.Marshal(map[string]any{
		"name":        "Ticket War",
		"total_stock": 100,
		"starts_at":   now.Format(time.RFC3339),
		"ends_at":     now.Add(time.Hour).Format(time.RFC3339),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/events", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// GET existing
	getReq := httptest.NewRequest(http.MethodGet, "/v1/events/11111111-1111-4111-8111-111111111111", nil)
	getRec := httptest.NewRecorder()
	router.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	// GET not found
	nfReq := httptest.NewRequest(http.MethodGet, "/v1/events/missing", nil)
	nfRec := httptest.NewRecorder()
	router.ServeHTTP(nfRec, nfReq)
	if nfRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", nfRec.Code)
	}
}

func TestOrderHandlers_ReservePayCancel_AndErrorMapping(t *testing.T) {
	orderSvc := &stubOrderService{
		reserveFn: func(_ context.Context, input service.ReserveOrderInput) (domain.ReserveOrderResult, error) {
			switch input.IdempotencyKey {
			case "replay-key":
				return domain.ReserveOrderResult{
					Order:    domain.Order{ID: "ord-1", Status: domain.OrderStatusReserved},
					Replayed: true,
				}, nil
			case "oos-key":
				return domain.ReserveOrderResult{}, domain.ErrOutOfStock
			case "qty-key":
				return domain.ReserveOrderResult{}, domain.ErrInvalidQuantity
			case "conflict-key":
				return domain.ReserveOrderResult{}, domain.ErrIdempotencyConflict
			default:
				return domain.ReserveOrderResult{
					Order:    domain.Order{ID: "ord-new", Status: domain.OrderStatusReserved},
					Replayed: false,
				}, nil
			}
		},
		payFn: func(_ context.Context, orderID string) (domain.Order, error) {
			if orderID == "expired-id" {
				return domain.Order{}, domain.ErrOrderExpired
			}
			return domain.Order{ID: orderID, Status: domain.OrderStatusPaid}, nil
		},
		cancelFn: func(_ context.Context, orderID string) (domain.Order, error) {
			if orderID == "paid-id" {
				return domain.Order{}, domain.ErrInvalidStateTransition
			}
			return domain.Order{ID: orderID, Status: domain.OrderStatusCancelled}, nil
		},
	}

	router := httptransport.NewRouter(discardLogger(), stubPinger{}, nil, nil, nil, orderSvc)

	// Missing headers -> 400 INVALID_INPUT
	reqNoHdr := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader([]byte(`{"event_id":"e1","quantity":1}`)))
	recNoHdr := httptest.NewRecorder()
	router.ServeHTTP(recNoHdr, reqNoHdr)
	if recNoHdr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing headers, got %d", recNoHdr.Code)
	}

	cases := []struct {
		name     string
		idemKey  string
		wantCode int
		wantErr  string
	}{
		{name: "new reservation", idemKey: "new-key", wantCode: http.StatusCreated},
		{name: "idempotent replay", idemKey: "replay-key", wantCode: http.StatusOK},
		{name: "out of stock", idemKey: "oos-key", wantCode: http.StatusConflict, wantErr: "OUT_OF_STOCK"},
		{name: "invalid quantity", idemKey: "qty-key", wantCode: http.StatusBadRequest, wantErr: "INVALID_QUANTITY"},
		{name: "idempotency conflict", idemKey: "conflict-key", wantCode: http.StatusConflict, wantErr: "IDEMPOTENCY_CONFLICT"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewReader([]byte(`{"event_id":"11111111-1111-4111-8111-111111111111","quantity":1}`)))
			req.Header.Set("X-User-ID", "user-1")
			req.Header.Set("Idempotency-Key", tc.idemKey)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("expected %d, got %d: %s", tc.wantCode, rec.Code, rec.Body.String())
			}
			if tc.wantErr != "" {
				var errResp struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
					t.Fatalf("unmarshal error body: %v", err)
				}
				if errResp.Error.Code != tc.wantErr {
					t.Fatalf("expected error code %q, got %q", tc.wantErr, errResp.Error.Code)
				}
			}
		})
	}

	// Pay success & expired
	payOK := httptest.NewRecorder()
	router.ServeHTTP(payOK, httptest.NewRequest(http.MethodPost, "/v1/orders/ord-1/pay", nil))
	if payOK.Code != http.StatusOK {
		t.Fatalf("expected 200 on pay, got %d", payOK.Code)
	}

	payExp := httptest.NewRecorder()
	router.ServeHTTP(payExp, httptest.NewRequest(http.MethodPost, "/v1/orders/expired-id/pay", nil))
	if payExp.Code != http.StatusConflict {
		t.Fatalf("expected 409 on expired pay, got %d", payExp.Code)
	}

	// Cancel success & invalid state transition
	cancelOK := httptest.NewRecorder()
	router.ServeHTTP(cancelOK, httptest.NewRequest(http.MethodPost, "/v1/orders/ord-1/cancel", nil))
	if cancelOK.Code != http.StatusOK {
		t.Fatalf("expected 200 on cancel, got %d", cancelOK.Code)
	}

	cancelPaid := httptest.NewRecorder()
	router.ServeHTTP(cancelPaid, httptest.NewRequest(http.MethodPost, "/v1/orders/paid-id/cancel", nil))
	if cancelPaid.Code != http.StatusConflict {
		t.Fatalf("expected 409 on cancel paid order, got %d", cancelPaid.Code)
	}
}
