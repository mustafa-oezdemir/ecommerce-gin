package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

const UnreadNotificationCountKey = "unread_notification_count"

func LoadUnreadNotificationCount(database *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if user, ok := CurrentUser(c); ok && user.Role == models.RoleCustomer {
			var count int64
			if err := database.WithContext(c.Request.Context()).Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", user.ID, false).Count(&count).Error; err == nil {
				c.Set(UnreadNotificationCountKey, count)
			}
		}
		c.Next()
	}
}
