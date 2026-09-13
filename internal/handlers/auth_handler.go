package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	data := gin.H{}
	if c.Query("status") == "registered" {
		data["success"] = "Your account is ready. Log in to start shopping."
	}
	c.HTML(http.StatusOK, "auth/login", viewData(c, data))
}

func (h *AuthHandler) ShowRegister(c *gin.Context) {
	c.HTML(http.StatusOK, "auth/register", viewData(c, nil))
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req validation.RegisterRequest
	if err := c.ShouldBind(&req); err != nil {
		h.renderRegister(c, http.StatusBadRequest, req, "Complete every field and use a password of at least 12 characters.")
		return
	}
	firstName := strings.TrimSpace(req.FirstName)
	lastName := strings.TrimSpace(req.LastName)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if utf8.RuneCountInString(firstName) < 1 || utf8.RuneCountInString(lastName) < 1 || email == "" {
		h.renderRegister(c, http.StatusBadRequest, req, "Enter a valid first name, last name, and email address.")
		return
	}
	if req.Password != req.PasswordConfirmation {
		h.renderRegister(c, http.StatusBadRequest, req, "The passwords do not match.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "registration password hashing failed", "error", err)
		h.renderRegister(c, http.StatusInternalServerError, req, "Your account could not be created. Please try again.")
		return
	}
	user := models.User{
		Name:      firstName + " " + lastName,
		FirstName: firstName,
		LastName:  lastName,
		Email:     email,
		Password:  string(hash),
		Role:      models.RoleCustomer,
	}
	if err := h.database.WithContext(c.Request.Context()).Create(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			h.renderRegister(c, http.StatusConflict, req, "An account with this email address already exists.")
			return
		}
		slog.ErrorContext(c.Request.Context(), "customer registration failed", "error", err)
		h.renderRegister(c, http.StatusInternalServerError, req, "Your account could not be created. Please try again.")
		return
	}
	slog.InfoContext(c.Request.Context(), "customer registered", "user_id", user.ID)
	c.Redirect(http.StatusSeeOther, "/login?status=registered")
}

func (h *AuthHandler) renderRegister(c *gin.Context, status int, req validation.RegisterRequest, message string) {
	c.HTML(status, "auth/register", viewData(c, gin.H{
		"error":     message,
		"FirstName": strings.TrimSpace(req.FirstName),
		"LastName":  strings.TrimSpace(req.LastName),
		"Email":     strings.ToLower(strings.TrimSpace(req.Email)),
	}))
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req validation.LoginRequest
	if err := c.ShouldBind(&req); err != nil {
		recordLoginResult("failure")
		c.HTML(http.StatusUnauthorized, "auth/login", viewData(c, gin.H{"error": "Invalid email or password"}))
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password

	var user models.User
	if err := h.database.WithContext(c.Request.Context()).Where("email = ?", email).First(&user).Error; err != nil {
		if metric := metrics.Default(); metric != nil {
			metric.LoginFailures.Inc()
			metric.LoginAttempts.WithLabelValues("failure").Inc()
		}
		c.HTML(http.StatusUnauthorized, "auth/login", viewData(c, gin.H{"error": "Invalid email or password"}))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		if metric := metrics.Default(); metric != nil {
			metric.LoginFailures.Inc()
			metric.LoginAttempts.WithLabelValues("failure").Inc()
		}
		c.HTML(http.StatusUnauthorized, "auth/login", viewData(c, gin.H{"error": "Invalid email or password"}))
		return
	}

	session := sessions.Default(c)
	session.Clear()
	if user.TwoFactorEnabled {
		recordLoginResult("two_factor_required")
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
	c.HTML(http.StatusOK, "auth/two-factor-challenge", viewData(c, nil))
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
		if metric := metrics.Default(); metric != nil {
			metric.TwoFactorChallenges.WithLabelValues("failure").Inc()
		}
		c.Header("Cache-Control", "no-store")
		c.HTML(http.StatusUnauthorized, "auth/two-factor-challenge", viewData(c, gin.H{"error": "Invalid or expired authentication code"}))
		return
	}
	session.Clear()
	if metric := metrics.Default(); metric != nil {
		metric.TwoFactorChallenges.WithLabelValues("success").Inc()
	}
	h.completeLogin(c, session, user)
}

func (h *AuthHandler) completeLogin(c *gin.Context, session sessions.Session, user *models.User) {
	session.Set(middleware.SessionUserIDKey, strconv.FormatUint(uint64(user.ID), 10))
	session.Set(middleware.SessionSecurityVersionKey, strconv.FormatUint(user.SecurityVersion, 10))
	if err := session.Save(); err != nil {
		c.String(http.StatusInternalServerError, "Could not create session")
		return
	}
	recordLoginResult("success")
	switch user.Role {
	case models.RoleAdmin:
		c.Redirect(http.StatusFound, "/admin/dashboard")
	case models.RoleEmployee:
		c.Redirect(http.StatusFound, "/employee/dashboard")
	default:
		c.Redirect(http.StatusFound, "/")
	}
}

func recordLoginResult(result string) {
	if metric := metrics.Default(); metric != nil {
		metric.LoginAttempts.WithLabelValues(result).Inc()
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
