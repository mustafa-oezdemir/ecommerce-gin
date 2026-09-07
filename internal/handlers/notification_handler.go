package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

type NotificationHandler struct{ database *gorm.DB }

func NewNotificationHandler(database *gorm.DB) *NotificationHandler {
	if database == nil {
		panic("handlers: database is required")
	}
	return &NotificationHandler{database: database}
}

func (h *NotificationHandler) List(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var notifications []models.Notification
	if err := h.database.WithContext(c.Request.Context()).Where("user_id = ?", user.ID).Order("created_at DESC").Limit(100).Find(&notifications).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not load notifications")
		return
	}
	c.HTML(http.StatusOK, "account/notifications/index", viewData(c, gin.H{"Notifications": notifications}))
}

func (h *NotificationHandler) MarkRead(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	now := time.Now().UTC()
	result := h.database.WithContext(c.Request.Context()).Model(&models.Notification{}).Where("id = ? AND user_id = ?", uint(id), user.ID).Updates(map[string]any{"is_read": true, "read_at": now})
	if result.Error != nil {
		c.String(http.StatusInternalServerError, "Could not update notification")
		return
	}
	if result.RowsAffected == 0 {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusSeeOther, "/account/notifications")
}

func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	now := time.Now().UTC()
	if err := h.database.WithContext(c.Request.Context()).Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", user.ID, false).Updates(map[string]any{"is_read": true, "read_at": now}).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not update notifications")
		return
	}
	c.Redirect(http.StatusSeeOther, "/account/notifications")
}
