package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"gorm.io/gorm"
)

type HealthHandler struct {
	ping    func(context.Context) error
	metrics *metrics.Metrics
}

func NewHealthHandler(database *gorm.DB, appMetrics *metrics.Metrics) *HealthHandler {
	return &HealthHandler{metrics: appMetrics, ping: func(ctx context.Context) error {
		sqlDB, err := database.DB()
		if err != nil {
			return err
		}
		return sqlDB.PingContext(ctx)
	}}
}

func (h *HealthHandler) Live(c *gin.Context) {
	if h.metrics != nil {
		h.metrics.HealthLive.Set(1)
	}
	c.JSON(http.StatusOK, gin.H{"status": "alive"})
}

func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		if h.metrics != nil {
			h.metrics.HealthReady.Set(0)
		}
		slog.Default().Warn("readiness check failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
		return
	}
	if h.metrics != nil {
		h.metrics.HealthReady.Set(1)
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
