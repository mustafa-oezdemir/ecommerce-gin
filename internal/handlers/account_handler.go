package handlers

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/middleware"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/models"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/services"
	"github.com/mustafa-oezdemir/ecommerce-gin/internal/validation"
	"github.com/pquerna/otp"
	"gorm.io/gorm"
)

type AccountHandler struct {
	database           *gorm.DB
	productListService *services.ProductListService
	securityService    *services.AccountSecurityService
}

func NewAccountHandler(database *gorm.DB, securityService *services.AccountSecurityService) *AccountHandler {
	if database == nil {
		panic("handlers: database is required")
	}
	if securityService == nil {
		panic("handlers: account security service is required")
	}
	return &AccountHandler{database: database, productListService: services.NewProductListService(database), securityService: securityService}
}

func (h *AccountHandler) Show(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	h.renderAccount(c, http.StatusOK, gin.H{"User": user})
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
	if _, err := h.securityService.UpdateProfile(c.Request.Context(), user.ID, req.FirstName, req.LastName); err != nil {
		h.renderAccount(c, http.StatusBadRequest, gin.H{"User": user, "error": "Could not update profile"})
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
	version, err := h.securityService.ChangePassword(c.Request.Context(), user.ID, req.CurrentPassword, req.NewPassword, req.Confirmation)
	if err != nil {
		h.renderAccount(c, http.StatusBadRequest, gin.H{"User": user, "error": "The password could not be changed. Check the current password and confirmation."})
		return
	}
	setSessionSecurityVersion(c, version)
	c.Redirect(http.StatusFound, "/account")
}

func (h *AccountHandler) RequestEmailChange(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.EmailChangeRequest
	if c.ShouldBind(&req) != nil {
		h.renderAccount(c, http.StatusBadRequest, gin.H{"User": user, "error": "Invalid email change request"})
		return
	}
	if err := h.securityService.RequestEmailChange(c.Request.Context(), user.ID, req.CurrentPassword, req.Email); err != nil {
		status, message, reason := emailChangeFailure(err)
		level := slog.LevelWarn
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		slog.Log(c.Request.Context(), level, "account security event", "event", "email_change_request_failed", "user_id", user.ID, "reason", reason)
		h.renderAccount(c, status, gin.H{"User": user, "error": message})
		return
	}
	h.renderAccount(c, http.StatusOK, gin.H{"User": user, "success": "A verification code was sent to the new email address."})
}

func emailChangeFailure(err error) (int, string, string) {
	switch {
	case errors.Is(err, services.ErrInvalidCredentials):
		return http.StatusBadRequest, "The current password is incorrect.", "invalid_credentials"
	case errors.Is(err, services.ErrSecurityInput):
		return http.StatusBadRequest, "Enter a valid email address that is different from your current address.", "invalid_input"
	case errors.Is(err, services.ErrEmailUnavailable):
		return http.StatusConflict, "That email address cannot be used.", "email_unavailable"
	case errors.Is(err, services.ErrSecurityCooldown):
		return http.StatusTooManyRequests, "Please wait before requesting another verification code.", "cooldown"
	default:
		return http.StatusServiceUnavailable, "The verification email could not be sent. Please try again later.", "internal_error"
	}
}

func (h *AccountHandler) ConfirmEmailChange(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.SecurityCodeRequest
	if c.ShouldBind(&req) != nil {
		h.renderAccount(c, http.StatusBadRequest, gin.H{"User": user, "error": "Invalid verification code"})
		return
	}
	version, err := h.securityService.ConfirmEmailChange(c.Request.Context(), user.ID, req.Code)
	if err != nil {
		h.renderAccount(c, http.StatusBadRequest, gin.H{"User": user, "error": "The verification code is invalid or expired."})
		return
	}
	setSessionSecurityVersion(c, version)
	c.Redirect(http.StatusFound, "/account")
}

func (h *AccountHandler) BeginTwoFactor(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	setup, err := h.securityService.BeginTwoFactor(c.Request.Context(), *user)
	if err != nil {
		h.renderAccount(c, http.StatusInternalServerError, gin.H{"User": user, "error": "Could not start two-factor setup"})
		return
	}
	h.renderTwoFactorSetup(c, user, setup, nil)
}

func (h *AccountHandler) ShowTwoFactorSetup(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	setup, err := h.securityService.PendingTwoFactor(c.Request.Context(), user.ID)
	if err != nil {
		c.Redirect(http.StatusFound, "/account")
		return
	}
	h.renderTwoFactorSetup(c, user, setup, nil)
}

func (h *AccountHandler) ConfirmTwoFactor(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.SecurityCodeRequest
	if c.ShouldBind(&req) != nil {
		c.String(http.StatusBadRequest, "Invalid authentication code")
		return
	}
	codes, version, err := h.securityService.ConfirmTwoFactor(c.Request.Context(), user.ID, req.Code)
	if err != nil {
		setup, _ := h.securityService.PendingTwoFactor(c.Request.Context(), user.ID)
		h.renderTwoFactorSetup(c, user, setup, gin.H{"error": "Invalid authentication code"})
		return
	}
	setSessionSecurityVersion(c, version)
	user.TwoFactorEnabled = true
	h.renderAccount(c, http.StatusOK, gin.H{"User": user, "RecoveryCodes": codes, "success": "Two-factor authentication is enabled. Save these recovery codes now; they will not be shown again."})
}

func (h *AccountHandler) RegenerateRecoveryCodes(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.PasswordConfirmationRequest
	if c.ShouldBind(&req) != nil {
		c.String(http.StatusBadRequest, "Invalid password")
		return
	}
	codes, version, err := h.securityService.RegenerateRecoveryCodes(c.Request.Context(), user.ID, req.CurrentPassword)
	if err != nil {
		h.renderAccount(c, http.StatusBadRequest, gin.H{"User": user, "error": "Recovery codes could not be regenerated."})
		return
	}
	setSessionSecurityVersion(c, version)
	h.renderAccount(c, http.StatusOK, gin.H{"User": user, "RecoveryCodes": codes, "success": "New recovery codes were generated. Previous codes are no longer valid."})
}

func (h *AccountHandler) DisableTwoFactor(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.DisableTwoFactorRequest
	if c.ShouldBind(&req) != nil {
		c.String(http.StatusBadRequest, "Invalid request")
		return
	}
	version, err := h.securityService.DisableTwoFactor(c.Request.Context(), user.ID, req.CurrentPassword, req.Code)
	if err != nil {
		h.renderAccount(c, http.StatusBadRequest, gin.H{"User": user, "error": "Two-factor authentication could not be disabled."})
		return
	}
	setSessionSecurityVersion(c, version)
	c.Redirect(http.StatusFound, "/account")
}

func (h *AccountHandler) DeleteAccount(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.PasswordConfirmationRequest
	if c.ShouldBind(&req) != nil || h.securityService.DeleteAccount(c.Request.Context(), user.ID, req.CurrentPassword) != nil {
		h.renderAccount(c, http.StatusBadRequest, gin.H{"User": user, "error": "The account could not be deleted."})
		return
	}
	session := sessions.Default(c)
	session.Clear()
	session.Options(sessions.Options{Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	_ = session.Save()
	c.Redirect(http.StatusFound, "/login")
}

func (h *AccountHandler) renderAccount(c *gin.Context, status int, extra gin.H) {
	c.HTML(status, "account.tmpl", viewData(c, extra))
}

func (h *AccountHandler) renderTwoFactorSetup(c *gin.Context, user *models.User, setup *services.TwoFactorSetup, extra gin.H) {
	if setup == nil {
		c.Redirect(http.StatusFound, "/account")
		return
	}
	key, err := otp.NewKeyFromURL(setup.URI)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not create QR code")
		return
	}
	image, err := key.Image(240, 240)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not create QR code")
		return
	}
	var buffer bytes.Buffer
	if png.Encode(&buffer, image) != nil {
		c.String(http.StatusInternalServerError, "Could not create QR code")
		return
	}
	data := gin.H{"User": user, "TwoFactorSecret": setup.Secret, "TwoFactorQR": "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())}
	for key, value := range extra {
		data[key] = value
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	c.HTML(http.StatusOK, "two_factor_setup.tmpl", viewData(c, data))
}

func setSessionSecurityVersion(c *gin.Context, version uint64) {
	session := sessions.Default(c)
	session.Set(middleware.SessionSecurityVersionKey, strconv.FormatUint(version, 10))
	_ = session.Save()
}

func (h *AccountHandler) ListProductLists(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	lists, err := h.productListService.List(c.Request.Context(), user.ID)
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load lists")
		return
	}
	c.HTML(http.StatusOK, "product_lists.tmpl", viewData(c, gin.H{"Lists": lists}))
}

func (h *AccountHandler) CreateProductList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var req validation.CreateProductListRequest
	if err := c.ShouldBind(&req); err != nil {
		c.String(http.StatusBadRequest, "Invalid list name")
		return
	}
	if err := h.productListService.Create(c.Request.Context(), user.ID, req.Name); err != nil {
		if errors.Is(err, services.ErrInvalidProductListInput) {
			c.String(http.StatusBadRequest, "Could not create list")
			return
		}
		c.String(http.StatusInternalServerError, "Could not create list")
		return
	}
	c.Redirect(http.StatusFound, "/account/lists")
}

func (h *AccountHandler) ShowProductList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductListIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	list, err := h.productListService.Get(c.Request.Context(), user.ID, uri.ID)
	if err != nil {
		if errors.Is(err, services.ErrProductListNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusInternalServerError, "Could not load list")
		return
	}
	products, err := h.productListService.AvailableProducts(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "Could not load products")
		return
	}
	c.HTML(http.StatusOK, "product_list_detail.tmpl", viewData(c, gin.H{"List": list, "Products": products}))
}

func (h *AccountHandler) AddProductToList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductListIDURI
	var req validation.AddProductToListRequest
	if err := c.ShouldBindUri(&uri); err != nil || c.ShouldBind(&req) != nil {
		c.String(http.StatusBadRequest, "Invalid product")
		return
	}
	if err := h.productListService.AddProduct(c.Request.Context(), user.ID, uri.ID, req.ProductID); err != nil {
		if errors.Is(err, services.ErrProductListNotFound) || errors.Is(err, services.ErrProductNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		if errors.Is(err, services.ErrInvalidProductListInput) {
			c.String(http.StatusBadRequest, "Could not add product to list")
			return
		}
		c.String(http.StatusInternalServerError, "Could not add product to list")
		return
	}
	c.Redirect(http.StatusFound, "/account/lists/"+c.Param("id"))
}

func (h *AccountHandler) RemoveProductFromList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductListProductURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.productListService.RemoveProduct(c.Request.Context(), user.ID, uri.ListID, uri.ProductID); err != nil {
		if errors.Is(err, services.ErrProductListNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusInternalServerError, "Could not remove product from list")
		return
	}
	c.Redirect(http.StatusFound, "/account/lists/"+c.Param("id"))
}

func (h *AccountHandler) DeleteProductList(c *gin.Context) {
	user, ok := middleware.CurrentUser(c)
	if !ok {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var uri validation.ProductListIDURI
	if err := c.ShouldBindUri(&uri); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}
	if err := h.productListService.Delete(c.Request.Context(), user.ID, uri.ID); err != nil {
		if errors.Is(err, services.ErrProductListNotFound) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		c.String(http.StatusInternalServerError, "Could not delete list")
		return
	}
	c.Redirect(http.StatusFound, "/account/lists")
}
