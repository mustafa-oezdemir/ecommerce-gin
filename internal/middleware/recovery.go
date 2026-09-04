package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
)

func Recovery(logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.ErrorContext(context.Background(), "panic recovered",
					"request_id", c.GetString(RequestIDKey),
					"method", c.Request.Method,
					"route", routeLabel(c),
					"panic", fmt.Sprint(recovered),
					"stack", string(debug.Stack()),
				)
				if c.Writer.Written() {
					c.Abort()
					return
				}
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
