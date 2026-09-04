package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type loginAttempt struct {
	count   int
	resetAt time.Time
}

type LoginRateLimiter struct {
	mu          sync.Mutex
	entries     map[string]loginAttempt
	limit       int
	window      time.Duration
	now         func() time.Time
	lastCleanup time.Time
	maxEntries  int
}

func NewLoginRateLimiter(limit int, window time.Duration) *LoginRateLimiter {
	if limit < 1 || window <= 0 {
		panic("middleware: rate limit and window must be positive")
	}
	return &LoginRateLimiter{
		entries:    make(map[string]loginAttempt),
		limit:      limit,
		window:     window,
		now:        time.Now,
		maxEntries: 10_000,
	}
}

func (l *LoginRateLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := loginRateLimitKey(c.ClientIP(), c.PostForm("email"))
		now := l.now()
		l.mu.Lock()
		l.cleanupExpiredLocked(now)
		entry, exists := l.entries[key]
		if !exists || !now.Before(entry.resetAt) {
			entry = loginAttempt{resetAt: now.Add(l.window)}
		}
		if entry.count >= l.limit || (!exists && len(l.entries) >= l.maxEntries) {
			retryAfter := max(int(entry.resetAt.Sub(now).Seconds()+0.999), 1)
			if !exists && len(l.entries) >= l.maxEntries {
				retryAfter = max(int(l.window.Seconds()), 1)
			}
			setRateLimitHeaders(c, l.limit, 0, entry.resetAt)
			l.mu.Unlock()
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		entry.count++
		l.entries[key] = entry
		remaining := max(l.limit-entry.count, 0)
		setRateLimitHeaders(c, l.limit, remaining, entry.resetAt)
		l.mu.Unlock()
		c.Next()
	}
}

func (l *LoginRateLimiter) cleanupExpiredLocked(now time.Time) {
	cleanupInterval := min(l.window, time.Minute)
	if !l.lastCleanup.IsZero() && now.Sub(l.lastCleanup) < cleanupInterval {
		return
	}
	for key, entry := range l.entries {
		if !now.Before(entry.resetAt) {
			delete(l.entries, key)
		}
	}
	l.lastCleanup = now
}

func loginRateLimitKey(clientIP, email string) string {
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	digest := sha256.Sum256([]byte(normalizedEmail))
	return clientIP + "|" + hex.EncodeToString(digest[:])
}

func setRateLimitHeaders(c *gin.Context, limit, remaining int, resetAt time.Time) {
	c.Header("RateLimit-Limit", strconv.Itoa(limit))
	c.Header("RateLimit-Remaining", strconv.Itoa(remaining))
	c.Header("RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
}
