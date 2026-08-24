package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	HTTPRequestsTotal    *prometheus.CounterVec
	HTTPRequestDuration  *prometheus.HistogramVec
	HTTPRequestsInFlight prometheus.Gauge
	HTTPResponseSize     *prometheus.HistogramVec
	OrdersCreated        *prometheus.CounterVec
	CheckoutFailures     *prometheus.CounterVec
	OrderValueCents      prometheus.Histogram
	LoginFailures        prometheus.Counter
}

func New(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPRequestsTotal:    prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total", Help: "Total HTTP requests."}, []string{"method", "route", "status"}),
		HTTPRequestDuration:  prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "HTTP request duration in seconds."}, []string{"method", "route", "status"}),
		HTTPRequestsInFlight: prometheus.NewGauge(prometheus.GaugeOpts{Name: "http_requests_in_flight", Help: "HTTP requests currently in flight."}),
		HTTPResponseSize:     prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_response_size_bytes", Help: "HTTP response sizes in bytes."}, []string{"method", "route", "status"}),
		OrdersCreated:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ecommerce_orders_created_total", Help: "Orders created successfully."}, []string{"order_status"}),
		CheckoutFailures:     prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ecommerce_checkouts_failed_total", Help: "Failed checkout attempts."}, []string{"failure_reason"}),
		OrderValueCents:      prometheus.NewHistogram(prometheus.HistogramOpts{Name: "ecommerce_order_value_cents", Help: "Successful order values in cents.", Buckets: []float64{1000, 5000, 10000, 25000, 50000, 100000, 250000}}),
		LoginFailures:        prometheus.NewCounter(prometheus.CounterOpts{Name: "ecommerce_login_failures_total", Help: "Failed login attempts."}),
	}
	registerer.MustRegister(m.HTTPRequestsTotal, m.HTTPRequestDuration, m.HTTPRequestsInFlight, m.HTTPResponseSize, m.OrdersCreated, m.CheckoutFailures, m.OrderValueCents, m.LoginFailures)
	return m
}

var (
	defaultMu      sync.RWMutex
	defaultMetrics *Metrics
)

func SetDefault(m *Metrics) { defaultMu.Lock(); defaultMetrics = m; defaultMu.Unlock() }
func Default() *Metrics     { defaultMu.RLock(); defer defaultMu.RUnlock(); return defaultMetrics }
