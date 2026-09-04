package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestStackRecoversAndObservesPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := prometheus.NewRegistry()
	appMetrics := metrics.New(registry)
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	router := gin.New()
	Install(router, StackConfig{Logger: logger, MetricsRegistry: appMetrics})
	router.GET("/panic/:id", func(*gin.Context) { panic("test panic") })

	request := httptest.NewRequest(http.MethodGet, "/panic/123?token=secret", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	if response.Header().Get(RequestIDHeader) == "" {
		t.Fatal("expected a response request ID")
	}
	if !strings.Contains(logs.String(), `"route":"/panic/:id"`) || !strings.Contains(logs.String(), `"status":500`) {
		t.Fatalf("expected structured panic and request logs, got %s", logs.String())
	}
	if strings.Contains(logs.String(), "token=secret") || strings.Contains(logs.String(), "/panic/123") {
		t.Fatalf("logs contain raw request data: %s", logs.String())
	}

	metricsResponse := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	metricBody := metricsResponse.Body.String()
	if !strings.Contains(metricBody, `route="/panic/:id",status="500"`) {
		t.Fatalf("panic request was not measured: %s", metricBody)
	}
	if !strings.Contains(metricBody, "http_requests_in_flight 0") {
		t.Fatalf("in-flight gauge was not decremented: %s", metricBody)
	}
}

func TestStackAppliesSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	Install(router, StackConfig{})
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := response.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("expected X-Frame-Options DENY, got %q", got)
	}
	if got := response.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
		t.Fatalf("expected COOP same-origin, got %q", got)
	}
	csp := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "script-src-attr 'none'") {
		t.Fatalf("expected CSP to allow self-hosted scripts and reject inline handlers, got %q", csp)
	}
}
