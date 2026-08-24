package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/db"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"golang.org/x/crypto/bcrypt"
)

type AccountHandler struct{}

func NewAccountHandler() *AccountHandler { return &AccountHandler{} }

func (h *AccountHandler) Show(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.HTML(http.StatusOK, "account.tmpl", viewData(c, gin.H{"User": user}))
}

func (h *AccountHandler) UpdateProfile(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.UpdateProfileRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid profile data")
		return
	}
	name, email := strings.TrimSpace(req.Name), strings.ToLower(strings.TrimSpace(req.Email))
	if name == "" || email == "" {
		c.String(http.StatusBadRequest, "Invalid profile data")
		return
	}
	if err := db.DB.Model(user).Updates(map[string]any{"name": name, "email": email}).Error; err != nil {
		c.String(http.StatusConflict, "Could not update profile")
		return
	}
	c.Redirect(http.StatusFound, "/account")
}

func (h *AccountHandler) ChangePassword(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.ChangePasswordRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid password data")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.CurrentPassword)); err != nil {
		c.String(http.StatusBadRequest, "Could not change password")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not change password")
		return
	}
	if err := db.DB.Model(user).Update("password", string(hash)).Error; err != nil {
		c.String(http.StatusInternalServerError, "Could not change password")
		return
	}
	c.Redirect(http.StatusFound, "/account")
}
