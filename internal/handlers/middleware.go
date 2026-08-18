package handlers

import (
    "fmt"
    "net/http"
    "strconv"

    "github.com/gin-gonic/gin"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
)

func AuthRequired(role models.Role) gin.HandlerFunc {
    return func(c *gin.Context) {
        userIDStr, err := c.Cookie("user_id")
        if err != nil {
            c.Redirect(http.StatusFound, "/login")
            c.Abort()
            return
        }

        id, _ := strconv.Atoi(userIDStr)
        var user models.User
        if err := db.DB.First(&user, id).Error; err != nil {
            c.Redirect(http.StatusFound, "/login")
            c.Abort()
            return
        }

        if user.Role != role {
            c.String(http.StatusForbidden, "Forbidden")
            c.Abort()
            return
        }

        c.Set("currentUser", user)
        c.Next()
    }
}
