package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	appmetrics "github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
)

type StackConfig struct {
	Logger          *slog.Logger
	MetricsRegistry *appmetrics.Metrics
	EnableHSTS      bool
	Session         gin.HandlerFunc
}

// Install registers the global middleware in an intentional order. Observability
// wraps recovery so panics are returned as HTTP 500 responses and still measured.
func Install(engine *gin.Engine, config StackConfig) {
	handlers := []gin.HandlerFunc{
		RequestID(),
		RequestLogger(config.Logger),
	}
	if config.MetricsRegistry != nil {
		handlers = append(handlers, Metrics(config.MetricsRegistry))
	}
	handlers = append(handlers,
		SecurityHeaders(config.EnableHSTS),
		Recovery(config.Logger),
	)
	if config.Session != nil {
		handlers = append(handlers, config.Session)
	}
	engine.Use(handlers...)
}
