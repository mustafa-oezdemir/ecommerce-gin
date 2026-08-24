package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HealthHandler struct{ ping func(context.Context) error }

func NewHealthHandler(database *gorm.DB) *HealthHandler {
	return &HealthHandler{ping: func(ctx context.Context) error {
		sqlDB, err := database.DB()
		if err != nil {
			return err
		}
		return sqlDB.PingContext(ctx)
	}}
}

func (h *HealthHandler) Live(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "alive"}) }

func (h *HealthHandler) Ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.ping(ctx); err != nil {
		slog.Default().Warn("readiness check failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ready"})
}
