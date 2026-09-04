package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLoginRateLimiterNormalizesIdentityAndResets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := NewLoginRateLimiter(2, 2*time.Minute)
	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	limiter.now = func() time.Time { return now }

	router := gin.New()
	router.POST("/login", limiter.Middleware(), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	first := performLogin(router, " User@Example.com ")
	if first.Code != http.StatusNoContent || first.Header().Get("RateLimit-Remaining") != "1" {
		t.Fatalf("unexpected first response: status=%d remaining=%q", first.Code, first.Header().Get("RateLimit-Remaining"))
	}
	second := performLogin(router, "user@example.com")
	if second.Code != http.StatusNoContent || second.Header().Get("RateLimit-Remaining") != "0" {
		t.Fatalf("unexpected second response: status=%d remaining=%q", second.Code, second.Header().Get("RateLimit-Remaining"))
	}
	blocked := performLogin(router, "USER@example.com")
	if blocked.Code != http.StatusTooManyRequests || blocked.Header().Get("Retry-After") != "120" {
		t.Fatalf("unexpected blocked response: status=%d retry=%q", blocked.Code, blocked.Header().Get("Retry-After"))
	}

	now = now.Add(2 * time.Minute)
	afterReset := performLogin(router, "user@example.com")
	if afterReset.Code != http.StatusNoContent || afterReset.Header().Get("RateLimit-Remaining") != "1" {
		t.Fatalf("unexpected response after reset: status=%d remaining=%q", afterReset.Code, afterReset.Header().Get("RateLimit-Remaining"))
	}
}

func performLogin(router http.Handler, email string) *httptest.ResponseRecorder {
	form := url.Values{"email": {email}}
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
