package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

func TestNewRouterBuildsApplicationRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, err := NewRouter(RouterConfig{
		Environment:   "test",
		SessionSecret: "a-session-secret-that-is-long-enough",
		CSRFKey:       []byte("12345678901234567890123456789012"),
		Database:      &gorm.DB{},
		Metrics:       metrics.New(prometheus.NewRegistry()),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"status":"alive"}` {
		t.Fatalf("unexpected liveness response: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get(middleware.RequestIDHeader) == "" {
		t.Fatal("server router did not install the middleware stack")
	}
}

func TestNewRouterRequiresDependencies(t *testing.T) {
	_, err := NewRouter(RouterConfig{Environment: "test", SessionSecret: "a-session-secret-that-is-long-enough", CSRFKey: []byte("12345678901234567890123456789012")})
	if err == nil {
		t.Fatal("expected missing database to fail")
	}
}
