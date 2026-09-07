package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

func TestManagementRoles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name string
		user *models.User
		want int
	}{
		{name: "employee", user: &models.User{Role: models.RoleEmployee}, want: http.StatusNoContent},
		{name: "admin", user: &models.User{Role: models.RoleAdmin}, want: http.StatusNoContent},
		{name: "customer", user: &models.User{Role: models.RoleCustomer}, want: http.StatusForbidden},
		{name: "unauthenticated", want: http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/filtered", func(c *gin.Context) {
				if tt.user != nil {
					c.Set(CurrentUserKey, tt.user)
				}
			}, RequireRoles(models.RoleAdmin, models.RoleEmployee), func(c *gin.Context) {
				c.Status(http.StatusNoContent)
			})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/filtered", nil))
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d", response.Code, tt.want)
			}
		})
	}
}
