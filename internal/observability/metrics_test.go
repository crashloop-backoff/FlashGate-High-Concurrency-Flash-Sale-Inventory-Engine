package observability_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/crashloop-backoff/FlashGate-High-Concurrency-Flash-Sale-Inventory-Engine/internal/observability"
)

func TestNewMetrics_RegistersAllREDMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m, err := observability.NewMetrics(reg)
	if err != nil {
		t.Fatalf("unexpected error registering metrics: %v", err)
	}

	m.HTTPRequestsTotal.WithLabelValues("GET", "/health", "200").Inc()
	m.HTTPRequestDuration.WithLabelValues("GET", "/health").Observe(0.01)
	m.HTTPInflight.Set(1)
	m.RecordReservation("success")
	m.AddSweeperExpired(2)

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("unexpected error gathering metrics: %v", err)
	}

	found := make(map[string]bool, len(families))
	for _, f := range families {
		found[f.GetName()] = true
	}

	for _, name := range []string{
		"flashgate_http_requests_total",
		"flashgate_http_request_duration_seconds",
		"flashgate_http_inflight_requests",
		"flashgate_reservations_total",
		"flashgate_sweeper_expired_total",
	} {
		if !found[name] {
			t.Errorf("expected metric %q to be gathered", name)
		}
	}
}
