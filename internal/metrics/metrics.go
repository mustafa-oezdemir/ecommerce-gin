package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	HTTPRequestsTotal        *prometheus.CounterVec
	HTTPRequestDuration      *prometheus.HistogramVec
	HTTPRequestsInFlight     prometheus.Gauge
	HTTPResponseSize         *prometheus.HistogramVec
	OrdersCreated            *prometheus.CounterVec
	CheckoutFailures         *prometheus.CounterVec
	CheckoutStarted          prometheus.Counter
	CheckoutCompleted        prometheus.Counter
	PaymentFailures          prometheus.Counter
	StockConflicts           prometheus.Counter
	IdempotencyReplays       prometheus.Counter
	OrderValueCents          prometheus.Histogram
	LoginFailures            prometheus.Counter
	LoginAttempts            *prometheus.CounterVec
	TwoFactorChallenges      *prometheus.CounterVec
	CSRFRejections           prometheus.Counter
	RateLimitRejections      *prometheus.CounterVec
	HealthLive               prometheus.Gauge
	HealthReady              prometheus.Gauge
	ShippingAPIRequests      *prometheus.CounterVec
	ShippingAPIErrors        *prometheus.CounterVec
	ShippingAPIDuration      *prometheus.HistogramVec
	ShippingCallbacks        prometheus.Counter
	ShippingCallbackFailures prometheus.Counter
}

func New(registerer prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPRequestsTotal:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "http_requests_total", Help: "Total HTTP requests."}, []string{"method", "route", "status"}),
		HTTPRequestDuration:      prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_request_duration_seconds", Help: "HTTP request duration in seconds."}, []string{"method", "route", "status"}),
		HTTPRequestsInFlight:     prometheus.NewGauge(prometheus.GaugeOpts{Name: "http_requests_in_flight", Help: "HTTP requests currently in flight."}),
		HTTPResponseSize:         prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "http_response_size_bytes", Help: "HTTP response sizes in bytes."}, []string{"method", "route", "status"}),
		OrdersCreated:            prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ecommerce_orders_created_total", Help: "Orders created successfully."}, []string{"order_status"}),
		CheckoutFailures:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ecommerce_checkouts_failed_total", Help: "Failed checkout attempts."}, []string{"failure_reason"}),
		CheckoutStarted:          prometheus.NewCounter(prometheus.CounterOpts{Name: "ecommerce_checkout_started_total", Help: "Checkout submissions started."}),
		CheckoutCompleted:        prometheus.NewCounter(prometheus.CounterOpts{Name: "ecommerce_checkout_completed_total", Help: "Checkout submissions completed."}),
		PaymentFailures:          prometheus.NewCounter(prometheus.CounterOpts{Name: "ecommerce_payment_failed_total", Help: "Payment attempts that failed."}),
		StockConflicts:           prometheus.NewCounter(prometheus.CounterOpts{Name: "ecommerce_stock_conflict_total", Help: "Checkout stock conflicts."}),
		IdempotencyReplays:       prometheus.NewCounter(prometheus.CounterOpts{Name: "ecommerce_idempotency_replay_total", Help: "Checkout idempotency replays."}),
		OrderValueCents:          prometheus.NewHistogram(prometheus.HistogramOpts{Name: "ecommerce_order_value_cents", Help: "Successful order values in cents.", Buckets: []float64{1000, 5000, 10000, 25000, 50000, 100000, 250000}}),
		LoginFailures:            prometheus.NewCounter(prometheus.CounterOpts{Name: "ecommerce_login_failures_total", Help: "Failed login attempts."}),
		LoginAttempts:            prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ecommerce_login_attempts_total", Help: "Aggregate login outcomes."}, []string{"result"}),
		TwoFactorChallenges:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ecommerce_two_factor_challenges_total", Help: "Aggregate two-factor challenge outcomes."}, []string{"result"}),
		CSRFRejections:           prometheus.NewCounter(prometheus.CounterOpts{Name: "ecommerce_csrf_rejections_total", Help: "Requests rejected by CSRF validation."}),
		RateLimitRejections:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "ecommerce_rate_limit_rejections_total", Help: "Requests rejected by security rate limits."}, []string{"scope"}),
		HealthLive:               prometheus.NewGauge(prometheus.GaugeOpts{Name: "ecommerce_health_live", Help: "Whether the application process is live (1) or not (0)."}),
		HealthReady:              prometheus.NewGauge(prometheus.GaugeOpts{Name: "ecommerce_health_ready", Help: "Whether the application is ready to serve traffic (1) or not (0)."}),
		ShippingAPIRequests:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: "shipping_api_requests_total", Help: "E-Commerce requests to Shipping Service."}, []string{"method", "route", "status"}),
		ShippingAPIErrors:        prometheus.NewCounterVec(prometheus.CounterOpts{Name: "shipping_api_errors_total", Help: "E-Commerce requests to Shipping Service that failed."}, []string{"method", "route", "kind"}),
		ShippingAPIDuration:      prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "shipping_api_duration_seconds", Help: "E-Commerce to Shipping Service request duration."}, []string{"method", "route"}),
		ShippingCallbacks:        prometheus.NewCounter(prometheus.CounterOpts{Name: "shipping_callback_total", Help: "Shipping callbacks received by E-Commerce."}),
		ShippingCallbackFailures: prometheus.NewCounter(prometheus.CounterOpts{Name: "shipping_callback_failures_total", Help: "Shipping callbacks rejected or not processed."}),
	}
	registerer.MustRegister(m.HTTPRequestsTotal, m.HTTPRequestDuration, m.HTTPRequestsInFlight, m.HTTPResponseSize, m.OrdersCreated, m.CheckoutFailures, m.CheckoutStarted, m.CheckoutCompleted, m.PaymentFailures, m.StockConflicts, m.IdempotencyReplays, m.OrderValueCents, m.LoginFailures, m.LoginAttempts, m.TwoFactorChallenges, m.CSRFRejections, m.RateLimitRejections, m.HealthLive, m.HealthReady, m.ShippingAPIRequests, m.ShippingAPIErrors, m.ShippingAPIDuration, m.ShippingCallbacks, m.ShippingCallbackFailures)
	return m
}

var (
	defaultMu      sync.RWMutex
	defaultMetrics *Metrics
)

func SetDefault(m *Metrics) { defaultMu.Lock(); defaultMetrics = m; defaultMu.Unlock() }
func Default() *Metrics     { defaultMu.RLock(); defer defaultMu.RUnlock(); return defaultMetrics }
