package handlers

import (
    "fmt"
    "net/http"
    "github.com/gin-gonic/gin"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
    "github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
    "golang.org/x/crypto/bcrypt"
)

type AuthHandler struct{}

func NewAuthHandler() *AuthHandler {
    return &AuthHandler{}
}

func (h *AuthHandler) ShowLogin(c *gin.Context) {
    c.HTML(http.StatusOK, "login.tmpl", gin.H{})
}

func (h *AuthHandler) Login(c *gin.Context) {
    email := c.PostForm("email")
    password := c.PostForm("password")

    var user models.User
    if err := db.DB.Where("email = ?", email).First(&user).Error; err != nil {
        c.HTML(http.StatusUnauthorized, "login.tmpl", gin.H{"error": "Invalid credentials"})
        return
    }

    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
        c.HTML(http.StatusUnauthorized, "login.tmpl", gin.H{"error": "Invalid credentials"})
        return
    }

    // Basit session için cookie (gerçekte secure session kullanmak daha iyi)
    c.SetCookie("user_id", fmt.Sprintf("%d", user.ID), 3600, "/", "", false, true)
    c.Redirect(http.StatusFound, "/")
}

func (h *AuthHandler) Logout(c *gin.Context) {
    c.SetCookie("user_id", "", -1, "/", "", false, true)
    c.Redirect(http.StatusFound, "/")
}
