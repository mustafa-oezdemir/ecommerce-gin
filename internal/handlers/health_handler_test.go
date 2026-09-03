package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestHealthLiveAndReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := prometheus.NewRegistry()
	appMetrics := metrics.New(registry)
	ready := &HealthHandler{ping: func(context.Context) error { return nil }, metrics: appMetrics}
	r := gin.New()
	r.GET("/health/live", ready.Live)
	r.GET("/health/ready", ready.Ready)
	for _, path := range []string{"/health/live", "/health/ready"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
	if body := gatherMetrics(t, registry); !strings.Contains(body, "ecommerce_health_live 1") || !strings.Contains(body, "ecommerce_health_ready 1") {
		t.Fatalf("healthy metrics not exported correctly: %s", body)
	}

	notReady := &HealthHandler{ping: func(context.Context) error { return errors.New("database unavailable") }, metrics: appMetrics}
	w := httptest.NewRecorder()
	r2 := gin.New()
	r2.GET("/health/ready", notReady.Ready)
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if w.Code != http.StatusServiceUnavailable || w.Body.String() != "{\"status\":\"not_ready\"}" {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	if body := gatherMetrics(t, registry); !strings.Contains(body, "ecommerce_health_ready 0") {
		t.Fatalf("unhealthy readiness metric not exported correctly: %s", body)
	}
}

func gatherMetrics(t *testing.T, registry *prometheus.Registry) string {
	t.Helper()
	w := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return w.Body.String()
}
