package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/csrf"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
)

func viewData(c *gin.Context, data gin.H) gin.H {
	if data == nil {
		data = gin.H{}
	}
	data["CSRFField"] = csrf.TemplateField(c.Request)
	data["CSRFToken"] = csrf.Token(c.Request)
	data["ActiveNav"] = activeNavigation(c.Request.URL.Path)
	if user, ok := middleware.CurrentUser(c); ok {
		data["CurrentUser"] = user
	}
	if count, ok := c.Get(middleware.UnreadNotificationCountKey); ok {
		data["UnreadNotificationCount"] = count
	}
	return data
}

func activeNavigation(path string) string {
	switch {
	case strings.HasPrefix(path, "/admin/dashboard"):
		return "dashboard"
	case strings.HasPrefix(path, "/admin/users"):
		return "users"
	case strings.HasPrefix(path, "/admin/orders"):
		return "orders"
	case strings.HasPrefix(path, "/admin/categories"):
		return "categories"
	case strings.HasPrefix(path, "/admin/logs"):
		return "logs"
	case strings.HasPrefix(path, "/employee/dashboard"):
		return "dashboard"
	case strings.HasPrefix(path, "/employee/products"):
		return "products"
	case strings.HasPrefix(path, "/employee/orders"):
		return "orders"
	case strings.HasPrefix(path, "/account/orders"):
		return "orders"
	case strings.HasPrefix(path, "/account/notifications"):
		return "notifications"
	case strings.HasPrefix(path, "/cart") || strings.HasPrefix(path, "/checkout"):
		return "cart"
	case strings.HasPrefix(path, "/account"):
		return "account"
	default:
		return "products"
	}
}
