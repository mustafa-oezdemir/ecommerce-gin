package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type loginAttempt struct {
	count   int
	resetAt time.Time
}

type LoginRateLimiter struct {
	mu      sync.Mutex
	entries map[string]loginAttempt
	limit   int
	window  time.Duration
}

func NewLoginRateLimiter(limit int, window time.Duration) *LoginRateLimiter {
	return &LoginRateLimiter{entries: make(map[string]loginAttempt), limit: limit, window: window}
}

func (l *LoginRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP() + "|" + c.PostForm("email")
		now := time.Now()
		l.mu.Lock()
		entry := l.entries[key]
		if entry.resetAt.Before(now) {
			entry = loginAttempt{resetAt: now.Add(l.window)}
		}
		entry.count++
		l.entries[key] = entry
		l.mu.Unlock()
		if entry.count > l.limit {
			c.Header("Retry-After", "60")
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		c.Next()
	}
}
