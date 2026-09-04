package middleware

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"gorm.io/gorm"
)

const (
	SessionUserIDKey = "user_id"
	CurrentUserKey   = "currentUser"
)

func RequireAuth(database *gorm.DB) gin.HandlerFunc {
	if database == nil {
		panic("middleware: authentication database is required")
	}

	return func(c *gin.Context) {
		session := sessions.Default(c)
		value := session.Get(SessionUserIDKey)
		userIDText, ok := value.(string)
		if !ok || userIDText == "" {
			redirectToLogin(c)
			return
		}
		userID, err := strconv.ParseUint(userIDText, 10, 64)
		if err != nil || userID == 0 {
			clearSession(session)
			redirectToLogin(c)
			return
		}

		var user models.User
		if err := database.WithContext(c.Request.Context()).First(&user, uint(userID)).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				clearSession(session)
				redirectToLogin(c)
				return
			}
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		c.Set(CurrentUserKey, &user)
		c.Next()
	}
}

func CurrentUser(c *gin.Context) (*models.User, bool) {
	value, exists := c.Get(CurrentUserKey)
	user, ok := value.(*models.User)
	return user, exists && ok && user != nil
}

func redirectToLogin(c *gin.Context) {
	c.Redirect(http.StatusFound, "/login")
	c.Abort()
}

func clearSession(session sessions.Session) {
	session.Clear()
	session.Options(sessions.Options{Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	_ = session.Save()
}
