package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"

	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

const (
	SessionUserIDKey = "user_id"
	CurrentUserKey   = "currentUser"
)

func RequireAuth() gin.HandlerFunc {
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
		if err := db.DB.First(&user, uint(userID)).Error; err != nil {
			clearSession(session)
			redirectToLogin(c)
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
