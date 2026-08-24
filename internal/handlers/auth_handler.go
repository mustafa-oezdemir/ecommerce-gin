package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct{}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

func (h *AuthHandler) ShowLogin(c *gin.Context) {
	c.HTML(http.StatusOK, "login.tmpl", viewData(c, nil))
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req validation.LoginRequest
	if err := c.ShouldBind(&req); err != nil {
		c.HTML(http.StatusUnauthorized, "login.tmpl", viewData(c, gin.H{"error": "Invalid email or password"}))
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password

	var user models.User
	if err := db.DB.Where("email = ?", email).First(&user).Error; err != nil {
		if metric := metrics.Default(); metric != nil {
			metric.LoginFailures.Inc()
		}
		c.HTML(http.StatusUnauthorized, "login.tmpl", viewData(c, gin.H{"error": "Invalid email or password"}))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		if metric := metrics.Default(); metric != nil {
			metric.LoginFailures.Inc()
		}
		c.HTML(http.StatusUnauthorized, "login.tmpl", viewData(c, gin.H{"error": "Invalid email or password"}))
		return
	}

	session := sessions.Default(c)
	session.Clear()
	session.Set(middleware.SessionUserIDKey, strconv.FormatUint(uint64(user.ID), 10))
	if err := session.Save(); err != nil {
		c.String(http.StatusInternalServerError, "Could not create session")
		return
	}
	switch user.Role {
	case models.RoleAdmin:
		c.Redirect(http.StatusFound, "/admin/dashboard")
	case models.RoleEmployee:
		c.Redirect(http.StatusFound, "/employee/dashboard")
	default:
		c.Redirect(http.StatusFound, "/")
	}
}

func (h *AuthHandler) Logout(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Options(sessions.Options{Path: "/", MaxAge: -1, HttpOnly: true, Secure: c.Request.TLS != nil, SameSite: http.SameSiteLaxMode})
	if err := session.Save(); err != nil {
		c.String(http.StatusInternalServerError, "Could not destroy session")
		return
	}
	c.Redirect(http.StatusFound, "/login")
}
