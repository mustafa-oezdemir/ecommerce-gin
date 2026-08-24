package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

func RequireRoles(roles ...models.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := CurrentUser(c)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		for _, role := range roles {
			if user.Role == role {
				c.Next()
				return
			}
		}
		c.AbortWithStatus(http.StatusForbidden)
	}
}
