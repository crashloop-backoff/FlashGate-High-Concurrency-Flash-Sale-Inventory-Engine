package observability

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// DurationBuckets defines the histogram buckets for HTTP request latency in seconds.
var DurationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}

// Metrics holds all HTTP RED Prometheus metrics for FlashGate.
type Metrics struct {
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
	HTTPInflight        prometheus.Gauge
}

// NewMetrics creates and registers HTTP RED metrics on the provided Prometheus registerer.
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
	m := &Metrics{
		HTTPRequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "flashgate_http_requests_total",
				Help: "Total number of HTTP requests processed.",
			},
			[]string{"method", "route", "status_code"},
		),
		HTTPRequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "flashgate_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds.",
				Buckets: DurationBuckets,
			},
			[]string{"method", "route"},
		),
		HTTPInflight: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "flashgate_http_inflight_requests",
				Help: "Number of HTTP requests currently being processed.",
			},
		),
	}

	for _, c := range []prometheus.Collector{
		m.HTTPRequestsTotal,
		m.HTTPRequestDuration,
		m.HTTPInflight,
	} {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	return m, nil
}

// PoolStatProvider abstracts pgxpool.Pool.Stat() for testability.
type PoolStatProvider interface {
	Stat() *pgxpool.Stat
}

// DBPoolCollector exposes pgxpool connection statistics to Prometheus on each scrape.
type DBPoolCollector struct {
	pool          PoolStatProvider
	acquiredDesc  *prometheus.Desc
	idleDesc      *prometheus.Desc
	waitCountDesc *prometheus.Desc
}

// NewDBPoolCollector creates a Prometheus collector for pgxpool statistics.
func NewDBPoolCollector(pool PoolStatProvider) *DBPoolCollector {
	return &DBPoolCollector{
		pool: pool,
		acquiredDesc: prometheus.NewDesc(
			"flashgate_db_pool_acquired_conns",
			"Number of currently acquired pgxpool connections.",
			nil,
			nil,
		),
		idleDesc: prometheus.NewDesc(
			"flashgate_db_pool_idle_conns",
			"Number of currently idle pgxpool connections.",
			nil,
			nil,
		),
		waitCountDesc: prometheus.NewDesc(
			"flashgate_db_pool_wait_count_total",
			"Cumulative count of connection acquires that waited for an idle connection.",
			nil,
			nil,
		),
	}
}

// Describe sends metric descriptors to Prometheus.
func (c *DBPoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.acquiredDesc
	ch <- c.idleDesc
	ch <- c.waitCountDesc
}

// Collect reads current pgxpool stats and emits Prometheus metrics.
func (c *DBPoolCollector) Collect(ch chan<- prometheus.Metric) {
	if c.pool == nil {
		return
	}
	stat := c.pool.Stat()
	if stat == nil {
		return
	}

	ch <- prometheus.MustNewConstMetric(
		c.acquiredDesc,
		prometheus.GaugeValue,
		float64(stat.AcquiredConns()),
	)
	ch <- prometheus.MustNewConstMetric(
		c.idleDesc,
		prometheus.GaugeValue,
		float64(stat.IdleConns()),
	)
	ch <- prometheus.MustNewConstMetric(
		c.waitCountDesc,
		prometheus.CounterValue,
		float64(stat.EmptyAcquireCount()),
	)
}
