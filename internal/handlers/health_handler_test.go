package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHealthLiveAndReady(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ready := &HealthHandler{ping: func(context.Context) error { return nil }}
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
	notReady := &HealthHandler{ping: func(context.Context) error { return errors.New("database unavailable") }}
	w := httptest.NewRecorder()
	r2 := gin.New()
	r2.GET("/health/ready", notReady.Ready)
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if w.Code != http.StatusServiceUnavailable || w.Body.String() != "{\"status\":\"not_ready\"}" {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}
