package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestIDPreservesValidIncomingValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "edge:request-123")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if got := response.Header().Get(RequestIDHeader); got != "edge:request-123" {
		t.Fatalf("expected incoming request ID, got %q", got)
	}
}

func TestRequestIDReplacesUnsafeIncomingValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "unsafe request id")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	got := response.Header().Get(RequestIDHeader)
	if got == "unsafe request id" || !validRequestID(got) {
		t.Fatalf("expected a generated safe request ID, got %q", got)
	}
}
