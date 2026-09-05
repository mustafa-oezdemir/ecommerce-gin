package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/metrics"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	sessionTwoFactorUserID = "two_factor_user_id"
	sessionTwoFactorExpiry = "two_factor_expires"
)

type AuthHandler struct {
	database *gorm.DB
	security *services.AccountSecurityService
}

func NewAuthHandler(database *gorm.DB, security *services.AccountSecurityService) *AuthHandler {
	if database == nil {
		panic("handlers: database is required")
	}
	if security == nil {
		panic("handlers: account security service is required")
	}
	return &AuthHandler{database: database, security: security}
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
	if err := h.database.WithContext(c.Request.Context()).Where("email = ?", email).First(&user).Error; err != nil {
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
	if user.TwoFactorEnabled {
		session.Set(sessionTwoFactorUserID, strconv.FormatUint(uint64(user.ID), 10))
		session.Set(sessionTwoFactorExpiry, strconv.FormatInt(time.Now().Add(5*time.Minute).Unix(), 10))
		if err := session.Save(); err != nil {
			c.String(http.StatusInternalServerError, "Could not create two-factor challenge")
			return
		}
		c.Redirect(http.StatusFound, "/auth/two-factor-challenge")
		return
	}
	h.completeLogin(c, session, &user)
}

func (h *AuthHandler) ShowTwoFactorChallenge(c *gin.Context) {
	if _, ok := challengeUserID(sessions.Default(c)); !ok {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.HTML(http.StatusOK, "two_factor_challenge.tmpl", viewData(c, nil))
}

func (h *AuthHandler) VerifyTwoFactorChallenge(c *gin.Context) {
	session := sessions.Default(c)
	userID, ok := challengeUserID(session)
	if !ok {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	code := strings.TrimSpace(c.PostForm("code"))
	recovery := c.PostForm("method") == "recovery"
	user, err := h.security.VerifySecondFactor(c.Request.Context(), userID, code, recovery)
	if err != nil {
		c.Header("Cache-Control", "no-store")
		c.HTML(http.StatusUnauthorized, "two_factor_challenge.tmpl", viewData(c, gin.H{"error": "Invalid or expired authentication code"}))
		return
	}
	session.Clear()
	h.completeLogin(c, session, user)
}

func (h *AuthHandler) completeLogin(c *gin.Context, session sessions.Session, user *models.User) {
	session.Set(middleware.SessionUserIDKey, strconv.FormatUint(uint64(user.ID), 10))
	session.Set(middleware.SessionSecurityVersionKey, strconv.FormatUint(user.SecurityVersion, 10))
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

func challengeUserID(session sessions.Session) (uint, bool) {
	userValue, userOK := session.Get(sessionTwoFactorUserID).(string)
	expiresValue, expiresOK := session.Get(sessionTwoFactorExpiry).(string)
	userID, userErr := strconv.ParseUint(userValue, 10, 64)
	expires, expiresErr := strconv.ParseInt(expiresValue, 10, 64)
	if !userOK || !expiresOK || userErr != nil || expiresErr != nil || userID == 0 || time.Now().Unix() >= expires {
		return 0, false
	}
	return uint(userID), true
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
