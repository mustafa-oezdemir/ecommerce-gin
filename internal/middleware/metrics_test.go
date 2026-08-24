package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestMetricsUseRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := prometheus.NewRegistry()
	m := metrics.New(registry)
	r := gin.New()
	r.Use(Metrics(m))
	r.GET("/account/orders/:id", func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/account/orders/123", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)
	metricsRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recorder := httptest.NewRecorder()
	promhttp.HandlerFor(registry, promhttp.HandlerOpts{}).ServeHTTP(recorder, metricsRequest)
	body := recorder.Body.String()
	if !strings.Contains(body, `route="/account/orders/:id"`) || strings.Contains(body, "/account/orders/123") {
		t.Fatalf("unexpected metric route labels: %s", body)
	}
}
