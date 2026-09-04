package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	RequestIDKey    = "request_id"
	RequestIDHeader = "X-Request-ID"
)

var fallbackRequestIDSequence atomic.Uint64

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if !validRequestID(requestID) {
			requestID = newRequestID()
		}
		c.Set(RequestIDKey, requestID)
		c.Header(RequestIDHeader, requestID)
		c.Next()
	}
}

func RequestLogger(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		started := time.Now()
		defer func() {
			status := c.Writer.Status()
			if skipRequestLog(c, status) {
				return
			}

			level := slog.LevelInfo
			if status >= 500 {
				level = slog.LevelError
			} else if status >= 400 {
				level = slog.LevelWarn
			}
			logger.LogAttrs(context.Background(), level, "http request",
				slog.String("request_id", c.GetString(RequestIDKey)),
				slog.String("method", c.Request.Method),
				slog.String("route", routeLabel(c)),
				slog.Int("status", status),
				slog.Int64("duration_ms", time.Since(started).Milliseconds()),
				slog.Int("response_bytes", max(c.Writer.Size(), 0)),
			)
		}()
		c.Next()
	}
}

func validRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for i := 0; i < len(value); i++ {
		character := value[i]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return false
	}
	return true
}

func newRequestID() string {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err == nil {
		return hex.EncodeToString(randomBytes)
	}

	return strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.FormatUint(fallbackRequestIDSequence.Add(1), 36)
}

func skipRequestLog(c *gin.Context, status int) bool {
	if status >= 500 {
		return false
	}
	switch c.FullPath() {
	case "/health/live", "/health/ready", "/healthz", "/readyz":
		return true
	default:
		return false
	}
}
